package watcher

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

type diffBaseKey struct {
	branch string
	cwd    string
}

type DiffWatcher struct {
	store    *session.Store
	interval time.Duration
	window   time.Duration
	running  sync.Map // session.Id -> struct{}; one in-flight turn-diff per session
	polling  sync.Map // cwd -> struct{}; one in-flight poll per repo
	lastDiff sync.Map // gitDir -> string; last written uncommitted diff, to skip no-op writes

	baseMu    sync.Mutex
	baseByKey map[diffBaseKey]string
}

func NewDiffWatcher(store *session.Store, interval, window time.Duration) *DiffWatcher {
	return &DiffWatcher{
		store:     store,
		interval:  interval,
		window:    window,
		baseByKey: make(map[diffBaseKey]string),
	}
}

func (w *DiffWatcher) Run(ctx context.Context) error {
	if w.interval <= 0 {
		w.interval = time.Second
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case id := <-w.store.TurnAdded:
			sess, ok := w.store.GetById(id)
			if !ok || sess.Meta.CWD == "" {
				continue
			}
			if _, loaded := w.running.LoadOrStore(id, struct{}{}); loaded {
				continue
			}
			go w.refresh(ctx, id, sess.Meta.CWD)

		case <-ticker.C:
			w.pollAll(ctx)
		}
	}
}

// refresh recomputes the working-tree diff against the target branch for a single
// session, triggered when that session gets a new turn.
func (w *DiffWatcher) refresh(ctx context.Context, id session.Id, cwd string) {
	defer w.running.Delete(id)

	if !gitReady(ctx, cwd) {
		return
	}

	base := w.diffBase(ctx, cwd)
	output, err := gitDiff(ctx, cwd, "--merge-base", base)
	if err != nil {
		logDiffErr(string(id), "git diff", err)
		return
	}
	w.store.UpdateDiff(id, base, output)
	slog.Debug("DiffWatcher: refreshed diff", "session", id, "base", base, "bytes", len(output))
}

func (w *DiffWatcher) diffBase(ctx context.Context, cwd string) string {
	branch, err := gitOutput(ctx, cwd, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		branch = "HEAD"
	}

	key := diffBaseKey{branch: branch, cwd: cwd}
	w.baseMu.Lock()
	base, isCached := w.baseByKey[key]
	w.baseMu.Unlock()
	if isCached {
		return base
	}

	base = inferDiffBase(ctx, branch, cwd)
	w.baseMu.Lock()
	w.baseByKey[key] = base
	w.baseMu.Unlock()
	return base
}

// pollAll recomputes the live uncommitted diff (git diff HEAD) once per distinct
// active repo, skipping repos whose most recent session is older than the window.
func (w *DiffWatcher) pollAll(ctx context.Context) {
	seen := map[string]bool{}
	for _, sess := range w.store.List() {
		cwd := sess.Meta.CWD
		if cwd == "" || seen[cwd] {
			continue
		}
		if w.window > 0 && time.Since(sess.LastActive) > w.window {
			continue
		}
		seen[cwd] = true
		if _, busy := w.polling.LoadOrStore(cwd, struct{}{}); busy {
			continue
		}
		go w.pollRepo(ctx, cwd)
	}
}

// pollRepo computes git diff HEAD for one repo and, on change, writes the hook file
// and updates the in-memory diff for every session sharing that working directory.
func (w *DiffWatcher) pollRepo(ctx context.Context, cwd string) {
	defer w.polling.Delete(cwd)

	if !gitReady(ctx, cwd) {
		return
	}

	output, err := gitDiff(ctx, cwd, "HEAD")
	if err != nil {
		logDiffErr(cwd, "git diff HEAD", err)
		return
	}

	gitDir, err := gitOutput(ctx, cwd, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return
	}

	if prev, ok := w.lastDiff.Load(gitDir); ok && prev.(string) == output {
		return // unchanged since last tick — no file rewrite, no store churn
	}
	w.lastDiff.Store(gitDir, output)

	if err := writeFileAtomic(filepath.Join(gitDir, "peek-diff"), output); err != nil {
		slog.Warn("DiffWatcher: write peek-diff failed", "gitDir", gitDir, "err", err)
	}

	for _, sess := range w.store.List() {
		if sess.Meta.CWD == cwd {
			w.store.UpdateUncommittedDiff(sess.Meta.SessionId, output)
		}
	}
	slog.Debug("DiffWatcher: refreshed uncommitted diff", "cwd", cwd, "bytes", len(output))
}

func gitReady(ctx context.Context, cwd string) bool {
	if _, err := os.Stat(cwd); err != nil {
		return false
	}
	return gitSucceeds(ctx, cwd, "rev-parse", "--git-dir")
}

func inferDiffBase(ctx context.Context, branch, cwd string) string {
	if base := baseFromReflog(ctx, branch, cwd); base != "" {
		return base
	}
	if base := baseFromOriginHead(ctx, cwd); base != "" {
		return base
	}
	if base := baseFromLocalDefault(ctx, cwd); base != "" {
		return base
	}
	return "HEAD"
}

func baseFromReflog(ctx context.Context, branch, cwd string) string {
	if branch == "HEAD" {
		return ""
	}

	output, err := gitOutput(ctx, cwd, "reflog", "show", "--format=%gs", branch)
	if err != nil || output == "" {
		return ""
	}

	lines := strings.Split(output, "\n")
	oldest := lines[len(lines)-1]
	created, isCreationEntry := strings.CutPrefix(oldest, "branch: Created from ")
	if !isCreationEntry {
		return ""
	}

	base := strings.TrimPrefix(created, "refs/remotes/")
	base = strings.TrimPrefix(base, "refs/heads/")
	if base == "" || base == "HEAD" {
		return ""
	}
	if strings.HasPrefix(base, "claude/") {
		return ""
	}

	for _, ref := range []string{"refs/heads/" + base, "refs/remotes/" + base} {
		if gitSucceeds(ctx, cwd, "show-ref", "--verify", "--quiet", ref) {
			return base
		}
	}
	return ""
}

func baseFromOriginHead(ctx context.Context, cwd string) string {
	name, err := gitOutput(ctx, cwd, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}

	if !gitSucceeds(ctx, cwd, "rev-parse", "--verify", "--quiet", name) {
		return ""
	}
	return name
}

func baseFromLocalDefault(ctx context.Context, cwd string) string {
	for _, name := range []string{"main", "master"} {
		if gitSucceeds(ctx, cwd, "rev-parse", "--verify", "--quiet", "refs/heads/"+name) {
			return name
		}
	}
	return ""
}

func gitOutput(ctx context.Context, cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitSucceeds(ctx context.Context, cwd string, args ...string) bool {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	return cmd.Run() == nil
}

// excludedPaths lists dependency/generated directories and files excluded from diffs.
// These are common across language ecosystems and rarely useful for code review.
var excludedPaths = []string{
	// Go
	"vendor/",
	"go.sum",

	// JavaScript / TypeScript
	"node_modules/",
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"bun.lockb",

	// Python
	".venv/",
	"venv/",
	"*.egg-info/",
	"poetry.lock",
	"Pipfile.lock",

	// Ruby
	"Gemfile.lock",

	// PHP
	"composer.lock",

	// Rust
	"Cargo.lock",

	// .NET
	"packages/",

	// Dart / Flutter
	"pubspec.lock",
	".dart_tool/",

	// Generated / IDE
	"*.pb.go",
	"*.gen.go",
	"*.generated.*",
}

func gitDiff(ctx context.Context, cwd string, args ...string) (string, error) {
	cmdArgs := append([]string{"diff"}, args...)
	cmdArgs = append(cmdArgs, "--")
	cmdArgs = append(cmdArgs, ".")
	for _, pattern := range excludedPaths {
		cmdArgs = append(cmdArgs, ":!"+pattern)
	}
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	return string(out), err
}

func logDiffErr(ref, action string, err error) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		slog.Warn("DiffWatcher: "+action+" failed", "ref", ref, "stderr", string(exitErr.Stderr))
	} else {
		slog.Warn("DiffWatcher: "+action+" failed", "ref", ref, "err", err)
	}
}

func writeFileAtomic(path, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
