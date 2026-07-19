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

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
)

type DiffWatcher struct {
	store    *session.Store
	broker   *events.Broker
	target   string
	interval time.Duration
	window   time.Duration
	running  sync.Map // session.Id -> struct{}; one in-flight turn-diff per session
	polling  sync.Map // cwd -> struct{}; one in-flight poll per repo
	lastDiff sync.Map // gitDir -> string; last written uncommitted diff, to skip no-op writes
}

func NewDiffWatcher(store *session.Store, broker *events.Broker, target string, interval, window time.Duration) *DiffWatcher {
	return &DiffWatcher{
		store:    store,
		broker:   broker,
		target:   target,
		interval: interval,
		window:   window,
	}
}

func (w *DiffWatcher) Run(ctx context.Context) error {
	if w.interval <= 0 {
		w.interval = time.Second
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	ch, cancel := w.broker.Subscribe()
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case ev := <-ch:
			if ev.Type != events.TypeTurnAdded {
				continue
			}
			id := session.Id(ev.SessionId)
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

	output, err := gitDiff(ctx, cwd, w.target)
	if err != nil {
		logDiffErr(string(id), "git diff", err)
		return
	}
	w.store.UpdateDiff(id, w.target, output)
	slog.Debug("DiffWatcher: refreshed diff", "session", id, "target", w.target, "bytes", len(output))
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

	gitDir, err := gitAbsoluteDir(ctx, cwd)
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
	check := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	check.Dir = cwd
	return check.Run() == nil
}

func gitAbsoluteDir(ctx context.Context, cwd string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--absolute-git-dir")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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
