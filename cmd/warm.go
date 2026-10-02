package cmd

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/codex"
	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/kevinhorst/peek-mcp/tools"
	"github.com/kevinhorst/peek-mcp/watcher"
)

// warmDeps is what every warm period is built from; it lives as long as the process.
type warmDeps struct {
	broker       *events.Broker
	claudeHome   string
	codexHome    string
	coworkHome   string
	invocations  *tools.InvocationCounter
	pollInterval time.Duration
	pollWindow   time.Duration
	stateDir     *state.Dir
	store        *session.Store
}

func (d *warmDeps) coworkStoreDirs() []string {
	var dirs []string
	if d.coworkHome == "" {
		return dirs
	}

	for _, name := range coworkStoreNames {
		storeDir := filepath.Join(d.coworkHome, name)
		info, err := os.Stat(storeDir)
		isDir := err == nil && info.IsDir()
		if isDir {
			dirs = append(dirs, storeDir)
		}
	}
	return dirs
}

// restoreSnapshot loads the instance's store snapshot into the store and the watchers. It reports
// false when there is none or it does not load; the caller then starts from fresh watchers.
func (d *warmDeps) restoreSnapshot(watchers []*watcher.Watcher) bool {
	if d.stateDir == nil {
		return false
	}

	id := d.invocations.Id()
	file, err := d.stateDir.OpenInstanceStore(id)
	if err != nil {
		return false
	}
	defer d.stateDir.RemoveInstanceStore(id)
	defer file.Close()

	snapshot, err := session.ReadStoreSnapshot(bufio.NewReader(file))
	if err != nil {
		slog.Warn("warmDeps.restoreSnapshot: Failed to read snapshot", "instance", id, "err", err)
		return false
	}

	for _, transcripts := range watchers {
		if err := transcripts.RestoreFiles(snapshot.Files); err != nil {
			slog.Warn("warmDeps.restoreSnapshot: Failed to restore file states", "instance", id, "err", err)
			return false
		}
	}

	d.store.Restore(snapshot)
	return true
}

func (d *warmDeps) transcriptWatchers(window time.Duration) []*watcher.Watcher {
	var watchers []*watcher.Watcher
	newClaudeParser := func() watcher.Parser { return claude.NewParser() }

	if d.claudeHome != "" {
		watchedDir := filepath.Join(d.claudeHome, claude.ProjectsDir)
		watchers = append(watchers, watcher.New(session.AgentClaude, watchedDir, window, newClaudeParser, d.store))
	}

	for _, storeDir := range d.coworkStoreDirs() {
		coworkWatcher := watcher.New(session.AgentClaude, storeDir, window, newClaudeParser, d.store)
		coworkWatcher.TranscriptPathOk = isCoworkTranscriptPath
		coworkWatcher.Project = "cowork"
		watchers = append(watchers, coworkWatcher)
	}

	if d.codexHome != "" {
		watchedDir := filepath.Join(d.codexHome, codex.SessionDir)
		newCodexParser := func() watcher.Parser { return codex.NewParser() }
		watchers = append(watchers, watcher.New(session.AgentCodex, watchedDir, window, newCodexParser, d.store))
	}
	return watchers
}

func (d *warmDeps) writeSnapshot(watchers []*watcher.Watcher) {
	if d.stateDir == nil {
		return
	}

	files := make(map[string]session.FileState)
	for _, transcripts := range watchers {
		states, err := transcripts.FileStates()
		if err != nil {
			slog.Warn("warmDeps.writeSnapshot: Failed to collect file states", "err", err)
			return
		}
		maps.Copy(files, states)
	}

	err := d.stateDir.WriteInstanceStore(d.invocations.Id(), func(writer io.Writer) error {
		return d.store.WriteSnapshot(files, writer)
	})
	if err != nil {
		slog.Warn("warmDeps.writeSnapshot: Failed to write snapshot", "err", err)
	}
}

// warmSet is one warm period: the watchers, their context and their goroutines.
type warmSet struct {
	cancel   context.CancelFunc
	group    sync.WaitGroup
	watchers []*watcher.Watcher
}

func startWarmSet(ctx context.Context, deps *warmDeps, window time.Duration) *warmSet {
	startedAt := time.Now()
	warmCtx, cancel := context.WithCancel(ctx)
	set := &warmSet{cancel: cancel}

	set.watchers = deps.transcriptWatchers(window)
	if !deps.restoreSnapshot(set.watchers) {
		set.watchers = deps.transcriptWatchers(window)
	}

	var loads []<-chan struct{}
	for _, transcripts := range set.watchers {
		loads = append(loads, transcripts.Loaded())
		set.group.Go(func() { runWatcher(warmCtx, transcripts.Run, "transcript watcher") })
	}

	if deps.claudeHome != "" {
		plans := watcher.NewPlanWatcher(filepath.Join(deps.claudeHome, "plans"), deps.store)
		set.group.Go(func() { runWatcher(warmCtx, plans.Run, "plan watcher") })
	}

	if deps.codexHome != "" {
		index := watcher.NewCodexIndexWatcher(deps.codexHome, deps.store)
		loads = append(loads, index.Loaded())
		set.group.Go(func() { runWatcher(warmCtx, index.Run, "codex index watcher") })
	}

	diffs := watcher.NewDiffWatcher(
		deps.store,
		deps.broker,
		deps.pollInterval,
		deps.pollWindow,
		deps.stateDir,
	)
	set.group.Go(func() { runWatcher(warmCtx, diffs.Run, "diff watcher") })

	set.group.Go(func() { reportLoaded(warmCtx, deps, loads, startedAt) })
	return set
}

// stop ends the warm period: watchers closed, store written to the instance's snapshot file and dropped from memory.
func (s *warmSet) stop(deps *warmDeps) {
	s.cancel()
	s.group.Wait()

	deps.writeSnapshot(s.watchers)
	deps.store.Reset()
	debug.FreeOSMemory()
}

func reportLoaded(ctx context.Context, deps *warmDeps, loads []<-chan struct{}, startedAt time.Time) {
	if awaitInitialLoad(ctx, deps.store, loads, startedAt) {
		deps.invocations.SetState(tools.StateWarm)
	}
}

func runWatcher(ctx context.Context, loop func(ctx context.Context) error, name string) {
	err := loop(ctx)
	isFailure := err != nil && !errors.Is(err, context.Canceled)
	if isFailure {
		slog.Error("runWatcher: Watcher failed", "watcher", name, "err", err)
		os.Exit(1)
	}
}
