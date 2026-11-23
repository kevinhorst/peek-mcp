package watcher

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
)

// pinWindow bounds first-turn pinning to live sessions, so the startup backfill of old transcripts runs no git.
const pinWindow = time.Hour

// excludedPaths lists dependency and generated paths excluded from diffs.
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

type diffBaseKey struct {
	branch string
	cwd    string
}

// diffRequest is what a diff computation needs from the session, copied out under the store lock.
type diffRequest struct {
	agent      session.Agent
	base       string
	cwd        string
	id         session.Id
	lastActive time.Time
	target     string
}

// Differ computes a session's git diffs when a caller asks for them; nothing is polled or held in memory.
type Differ struct {
	baseByKey      map[diffBaseKey]string
	baseByKeyMutex sync.Mutex
	broker         *events.Broker
	pinAttempted   map[session.Id]struct{}
	stateDir       *state.Dir
	store          *session.Store
}

func NewDiffer(broker *events.Broker, store *session.Store, stateDir *state.Dir) *Differ {
	return &Differ{
		baseByKey:    make(map[diffBaseKey]string),
		broker:       broker,
		pinAttempted: make(map[session.Id]struct{}),
		stateDir:     stateDir,
		store:        store,
	}
}

func (d *Differ) diffBase(ctx context.Context, cwd string) string {
	branch, err := gitOutput(ctx, cwd, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		branch = "HEAD"
	}

	key := diffBaseKey{branch: branch, cwd: cwd}
	d.baseByKeyMutex.Lock()
	base, isCached := d.baseByKey[key]
	d.baseByKeyMutex.Unlock()
	if isCached {
		return base
	}

	base = inferDiffBase(ctx, branch, cwd)
	d.baseByKeyMutex.Lock()
	d.baseByKey[key] = base
	d.baseByKeyMutex.Unlock()
	return base
}

func (d *Differ) newRequest(id session.Id) (*diffRequest, bool) {
	var request *diffRequest
	found := d.store.WithSession(id, func(sess *session.Session) {
		request = &diffRequest{
			agent:      sess.Agent,
			base:       sess.DiffBase,
			cwd:        sess.Meta.CWD,
			id:         id,
			lastActive: sess.LastActive,
			target:     sess.DiffTarget,
		}
	})
	return request, found
}

// persistSnapshot keeps the last non-empty diff on disk so it outlives the working tree.
func (d *Differ) persistSnapshot(output string, request *diffRequest) {
	if d.stateDir == nil || output == "" {
		return
	}

	err := d.stateDir.WriteDiffSnapshot(string(request.agent), output, string(request.id))
	if err != nil {
		slog.Warn("Differ.persistSnapshot: Failed to write snapshot", "session", request.id, "err", err)
		return
	}

	d.store.MarkSnapshotPersisted(request.id)
}

// pinBase pins the merge-base as a SHA so target-branch advances, merges and deletions cannot move the session's diff.
func (d *Differ) pinBase(ctx context.Context, request *diffRequest) bool {
	if d.stateDir != nil {
		base, isPinned := d.stateDir.ReadDiffBase(string(request.agent), string(request.id))
		if isPinned {
			d.store.PinDiffBase(request.id, base.Sha, base.Target)
			request.base = base.Sha
			request.target = base.Target
			return true
		}
	}

	target := d.diffBase(ctx, request.cwd)
	sha, err := gitOutput(ctx, request.cwd, "merge-base", "--end-of-options", "HEAD", target)
	if err != nil {
		slog.Warn("Differ.pinBase: git merge-base failed", "session", request.id, "err", err)
		return false
	}

	d.store.PinDiffBase(request.id, sha, target)
	request.base = sha
	request.target = target
	if d.stateDir == nil {
		return true
	}

	err = d.stateDir.WriteDiffBase(string(request.agent), state.DiffBase{Sha: sha, Target: target}, string(request.id))
	if err != nil {
		slog.Warn("Differ.pinBase: Failed to write diff base", "session", request.id, "err", err)
	}
	return true
}

// pinFirstTurn pins a live session's base once, at the turn that introduces its working tree; Run is its only caller.
func (d *Differ) pinFirstTurn(ctx context.Context, id session.Id) {
	if _, isAttempted := d.pinAttempted[id]; isAttempted {
		return
	}

	request, found := d.newRequest(id)
	if !found || request.base != "" || request.cwd == "" {
		return
	}
	if time.Since(request.lastActive) > pinWindow {
		return
	}

	d.pinAttempted[id] = struct{}{}
	if !gitReady(ctx, request.cwd) {
		return
	}
	d.pinBase(ctx, request)
}

func (d *Differ) readSnapshot(request *diffRequest) string {
	if d.stateDir == nil {
		return ""
	}

	content, _, _ := d.stateDir.ReadDiffSnapshot(string(request.agent), string(request.id))
	return content
}

// Run pins each live session's diff base at its first turn, so a branch that lands before the first diff request keeps its committed work in the diff.
func (d *Differ) Run(ctx context.Context) error {
	turns, cancel := d.broker.Subscribe()
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-turns:
			if event.Type != events.TypeTurnAdded {
				continue
			}
			d.pinFirstTurn(ctx, session.Id(event.SessionId))
		}
	}
}

// SessionDiff diffs the working tree against the pinned merge-base, or serves the snapshot once the tree is gone.
func (d *Differ) SessionDiff(ctx context.Context, id session.Id) (content, target string, isFound bool) {
	request, isFound := d.newRequest(id)
	if !isFound {
		return "", "", false
	}

	if !gitReady(ctx, request.cwd) {
		return d.readSnapshot(request), request.target, true
	}

	if request.base == "" && !d.pinBase(ctx, request) {
		return d.readSnapshot(request), request.target, true
	}

	output, err := gitDiff(ctx, request.cwd, request.base)
	if err != nil {
		slog.Warn("Differ.SessionDiff: git diff failed", "session", id, "err", err)
		return d.readSnapshot(request), request.target, true
	}

	d.persistSnapshot(output, request)
	return output, request.target, true
}

// UncommittedDiff returns git diff HEAD in the session's working tree.
func (d *Differ) UncommittedDiff(ctx context.Context, id session.Id) (content string, isFound bool) {
	request, isFound := d.newRequest(id)
	if !isFound {
		return "", false
	}

	if !gitReady(ctx, request.cwd) {
		return "", true
	}

	output, err := gitDiff(ctx, request.cwd, "HEAD")
	if err != nil {
		slog.Warn("Differ.UncommittedDiff: git diff HEAD failed", "session", id, "err", err)
		return "", true
	}
	return output, true
}

func baseFromLocalDefault(ctx context.Context, cwd string) string {
	for _, name := range []string{"main", "master"} {
		if gitSucceeds(ctx, cwd, "rev-parse", "--verify", "--quiet", "refs/heads/"+name) {
			return name
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

// gitDiff stops reading at the snapshot cap; diff.autoRefreshIndex=false keeps git from rewriting the index under index.lock.
func gitDiff(ctx context.Context, cwd string, args ...string) (string, error) {
	cmdArgs := append([]string{"-c", "diff.autoRefreshIndex=false", "diff", "--no-ext-diff", "--end-of-options"}, args...)
	cmdArgs = append(cmdArgs, "--", ".")
	for _, pattern := range excludedPaths {
		cmdArgs = append(cmdArgs, ":!"+pattern)
	}

	diffCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(diffCtx, "git", cmdArgs...)
	cmd.Dir = cwd
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.Wrap(err, "gitDiff: Failed to open stdout")
	}

	err = cmd.Start()
	if err != nil {
		return "", errors.Wrap(err, "gitDiff: Failed to start git")
	}

	output, err := io.ReadAll(io.LimitReader(stdout, state.MaxSnapshotBytes+1))
	if err != nil {
		cancel()
		_ = cmd.Wait()
		return "", errors.Wrap(err, "gitDiff: Failed to read output")
	}

	if len(output) > state.MaxSnapshotBytes {
		cancel()
		_ = cmd.Wait()
		return state.Truncate(string(output)), nil
	}

	err = cmd.Wait()
	if err != nil {
		return "", errors.Wrapf(err, "gitDiff: %s", strings.TrimSpace(stderr.String()))
	}
	return string(output), nil
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

func gitReady(ctx context.Context, cwd string) bool {
	if cwd == "" {
		return false
	}
	if _, err := os.Stat(cwd); err != nil {
		return false
	}
	return gitSucceeds(ctx, cwd, "rev-parse", "--git-dir")
}

func gitSucceeds(ctx context.Context, cwd string, args ...string) bool {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	return cmd.Run() == nil
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
