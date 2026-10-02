package watcher

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
)

const (
	hookFileName = "peek-diff"
	hookFilePerm = 0o644
)

type diffBaseKey struct {
	branch string
	cwd    string
}

type DiffWatcher struct {
	store    *session.Store
	broker   *events.Broker
	interval time.Duration
	window   time.Duration
	stateDir *state.Dir
	running  sync.Map // session.Id -> struct{}; one in-flight turn-diff per session
	polling  sync.Map // cwd -> struct{}; one in-flight poll per repo
	lastDiff sync.Map // gitDir -> string; last written uncommitted diff, to skip no-op writes

	dirtyMu sync.Mutex
	dirty   map[session.Id]string // session -> cwd; turn-diff refreshes pending for the next tick

	baseMu    sync.Mutex
	baseByKey map[diffBaseKey]string

	gitDirMu    sync.Mutex
	gitDirByCwd map[string]string
}

func NewDiffWatcher(store *session.Store, broker *events.Broker, interval, window time.Duration, stateDir *state.Dir) *DiffWatcher {
	return &DiffWatcher{
		store:       store,
		broker:      broker,
		interval:    interval,
		window:      window,
		stateDir:    stateDir,
		dirty:       make(map[session.Id]string),
		baseByKey:   make(map[diffBaseKey]string),
		gitDirByCwd: make(map[string]string),
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
			if !w.isWithinWindow(sess) {
				continue
			}
			w.markDirty(id, sess.Meta.CWD)

		case <-ticker.C:
			w.refreshDirty(ctx)
			w.pollAll(ctx)
		}
	}
}

func (w *DiffWatcher) markDirty(id session.Id, cwd string) {
	w.dirtyMu.Lock()
	defer w.dirtyMu.Unlock()
	w.dirty[id] = cwd
}

// refreshDirty flushes turn-diff refreshes accumulated since the last tick —
// one refresh per session regardless of how many turns arrived in between.
func (w *DiffWatcher) refreshDirty(ctx context.Context) {
	w.dirtyMu.Lock()
	pending := w.dirty
	w.dirty = make(map[session.Id]string)
	w.dirtyMu.Unlock()

	for id, cwd := range pending {
		if _, loaded := w.running.LoadOrStore(id, struct{}{}); loaded {
			continue
		}
		go w.refresh(ctx, id, cwd)
	}
}

func (w *DiffWatcher) isWithinWindow(sess *session.Session) bool {
	if w.window <= 0 {
		return true
	}
	return time.Since(sess.LastActive) <= w.window
}

// refresh recomputes the working-tree diff against the session's pinned base,
// triggered when that session gets a new turn. Failures flip the served
// source to the persisted snapshot instead of silently keeping stale state.
func (w *DiffWatcher) refresh(ctx context.Context, id session.Id, cwd string) {
	defer w.running.Delete(id)

	sess, ok := w.store.GetById(id)
	if !ok {
		return
	}

	if !gitReady(ctx, cwd) {
		w.store.MarkDiffSnapshot(id)
		return
	}

	base := sess.DiffBase
	target := sess.DiffTarget
	if base == "" {
		pinnedBase, pinnedTarget, isPinned := w.pinBase(ctx, cwd, id)
		if !isPinned {
			w.store.MarkDiffSnapshot(id)
			return
		}
		base = pinnedBase
		target = pinnedTarget
	}

	existing, isFresh := w.readSnapshot(sess)
	if isFresh {
		w.store.UpdateDiff(id, target, existing)
		w.store.MarkSnapshotPersisted(id, existing)
		return
	}

	output, err := gitDiff(ctx, cwd, base)
	if err != nil {
		logDiffErr(string(id), "git diff", err)
		w.store.MarkDiffSnapshot(id)
		return
	}

	w.store.UpdateDiff(id, target, output)
	w.persistSnapshot(output, existing, sess)
	slog.Debug("DiffWatcher: refreshed diff", "session", id, "base", base, "bytes", len(output))
}

// pinBase resolves the target branch once (existing inference) and pins the
// merge-base as a SHA, so later target-branch advances, merges, and branch
// deletions cannot move or collapse the session's diff.
func (w *DiffWatcher) pinBase(ctx context.Context, cwd string, id session.Id) (sha, target string, ok bool) {
	if base, isPinned := w.readBase(id); isPinned {
		w.store.PinDiffBase(id, base.Sha, base.Target)
		return base.Sha, base.Target, true
	}

	target = w.diffBase(ctx, cwd)
	sha, err := gitOutput(ctx, cwd, "merge-base", "--end-of-options", "HEAD", target)
	if err != nil {
		logDiffErr(string(id), "git merge-base", err)
		return "", "", false
	}

	w.store.PinDiffBase(id, sha, target)
	w.persistBase(id, sha, target)
	return sha, target, true
}

func (w *DiffWatcher) persistBase(id session.Id, sha, target string) {
	if w.stateDir == nil {
		return
	}

	sess, ok := w.store.GetById(id)
	if !ok {
		return
	}

	base := state.DiffBase{Sha: sha, Target: target}
	if err := w.stateDir.WriteDiffBase(string(sess.Agent), base, string(id)); err != nil {
		slog.Warn("DiffWatcher.persistBase: Failed to write diff base", "session", id, "err", err)
	}
}

// Empty outputs never overwrite the snapshot: an empty live diff is served
// live, but the last real work is retained for post-cleanup analysis.
func (w *DiffWatcher) persistSnapshot(output, previous string, sess *session.Session) {
	if w.stateDir == nil || output == "" {
		return
	}

	isUnchanged := state.Truncate(output) == previous
	if err := w.publishSnapshot(isUnchanged, output, sess); err != nil {
		slog.Warn("DiffWatcher.persistSnapshot: Failed to write snapshot", "session", sess.Meta.SessionId, "err", err)
		return
	}

	w.store.MarkSnapshotPersisted(sess.Meta.SessionId, output)
}

func (w *DiffWatcher) publishSnapshot(isUnchanged bool, output string, sess *session.Session) error {
	agent := string(sess.Agent)
	id := string(sess.Meta.SessionId)
	if isUnchanged {
		return w.stateDir.TouchDiffSnapshot(agent, id)
	}
	return w.stateDir.WriteDiffSnapshot(agent, output, id)
}

func (w *DiffWatcher) readBase(id session.Id) (state.DiffBase, bool) {
	var base state.DiffBase
	if w.stateDir == nil {
		return base, false
	}

	sess, ok := w.store.GetById(id)
	if !ok {
		return base, false
	}
	return w.stateDir.ReadDiffBase(string(sess.Agent), string(id))
}

// readSnapshot returns the persisted session diff and whether an instance wrote it after the session's latest turn.
func (w *DiffWatcher) readSnapshot(sess *session.Session) (content string, isFresh bool) {
	if w.stateDir == nil {
		return "", false
	}

	content, capturedAt, ok := w.stateDir.ReadDiffSnapshot(string(sess.Agent), string(sess.Meta.SessionId))
	if !ok {
		return "", false
	}
	return content, !capturedAt.Before(sess.LastActive)
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
		if !w.isWithinWindow(sess) {
			continue
		}
		seen[cwd] = true
		if _, busy := w.polling.LoadOrStore(cwd, struct{}{}); busy {
			continue
		}
		go w.pollRepo(ctx, cwd)
	}
}

// pollRepo keeps one repo's hook file and the sessions sharing that directory current.
func (w *DiffWatcher) pollRepo(ctx context.Context, cwd string) {
	defer w.polling.Delete(cwd)

	gitDir, ok := w.resolveGitDir(ctx, cwd)
	if !ok {
		return
	}

	output, ok := w.uncommittedDiff(ctx, cwd, filepath.Join(gitDir, hookFileName))
	if !ok {
		return
	}

	if prev, ok := w.lastDiff.Load(gitDir); ok && prev.(string) == output {
		return // unchanged since last tick — no store churn
	}
	w.lastDiff.Store(gitDir, output)

	truncated := state.Truncate(output)
	for _, sess := range w.store.List() {
		if sess.Meta.CWD == cwd {
			w.store.UpdateUncommittedDiff(sess.Meta.SessionId, truncated)
		}
	}
	slog.Debug("DiffWatcher: refreshed uncommitted diff", "cwd", cwd, "bytes", len(output))
}

func (w *DiffWatcher) resolveGitDir(ctx context.Context, cwd string) (string, bool) {
	w.gitDirMu.Lock()
	gitDir, isKnown := w.gitDirByCwd[cwd]
	w.gitDirMu.Unlock()
	if isKnown {
		return gitDir, true
	}

	if !gitReady(ctx, cwd) {
		return "", false
	}

	gitDir, err := gitOutput(ctx, cwd, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", false
	}

	w.gitDirMu.Lock()
	w.gitDirByCwd[cwd] = gitDir
	w.gitDirMu.Unlock()
	return gitDir, true
}

// uncommittedDiff returns the hook file's content when an instance refreshed it within the
// last interval; otherwise it computes git diff HEAD and publishes it to the hook file.
func (w *DiffWatcher) uncommittedDiff(ctx context.Context, cwd, hookPath string) (string, bool) {
	existing, modTime, hasFile := readHookFile(hookPath)
	isFresh := hasFile && time.Since(modTime) < w.interval
	if isFresh {
		return existing, true
	}

	if !gitReady(ctx, cwd) {
		return "", false
	}

	output, err := gitDiff(ctx, cwd, "HEAD")
	if err != nil {
		logDiffErr(cwd, "git diff HEAD", err)
		return "", false
	}

	isUnchanged := hasFile && existing == output
	publishHookFile(isUnchanged, output, hookPath)
	return output, true
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

	if !gitSucceeds(ctx, cwd, "rev-parse", "--verify", "--quiet", "--end-of-options", name) {
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
	cmdArgs := append([]string{"diff", "--no-ext-diff", "--end-of-options"}, args...)
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

func publishHookFile(isUnchanged bool, output, path string) {
	if isUnchanged {
		now := time.Now()
		if err := os.Chtimes(path, now, now); err != nil {
			slog.Warn("publishHookFile: Failed to advance timestamp", "path", path, "err", err)
		}
		return
	}

	if err := writeFileAtomic(path, output); err != nil {
		slog.Warn("publishHookFile: Failed to write hook file", "path", path, "err", err)
	}
}

func readHookFile(path string) (content string, modTime time.Time, ok bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", modTime, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", modTime, false
	}
	return string(data), info.ModTime(), true
}

func writeFileAtomic(path, content string) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return errors.Wrap(err, "writeFileAtomic: Failed to create temp file")
	}

	tmp := file.Name()
	_, err = file.WriteString(content)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmp, hookFilePerm)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return errors.Wrap(err, "writeFileAtomic: Failed to write file")
	}
	return nil
}
