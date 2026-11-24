package index

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/kevinhorst/peek-mcp/watcher"
)

// Window is how far back the index answers: fourteen days of session summaries, whatever the transcript watch window.
const Window = 14 * 24 * time.Hour

// backfillLockStale hands a dead backfiller's lock to the next instance.
const backfillLockStale = 30 * time.Minute

// backfillDepth keeps one turn per scope in the throwaway store a backfill parses into; the summary needs none.
const backfillDepth = 1

const transcriptSuffix = ".jsonl"

// Indexer keeps one summary record per Claude session in the state dir: live sessions from the store, older ones parsed once.
type Indexer struct {
	loader         *watcher.TurnLoader
	mu             sync.Mutex
	persistedUntil map[session.Id]time.Time
	projectsDir    string
	stateDir       *state.Dir
	store          *session.Store
}

func NewIndexer(loader *watcher.TurnLoader, projectsDir string, stateDir *state.Dir, store *session.Store) *Indexer {
	return &Indexer{
		loader:         loader,
		persistedUntil: make(map[session.Id]time.Time),
		projectsDir:    projectsDir,
		stateDir:       stateDir,
		store:          store,
	}
}

// backfillProject records every transcript of one project dir that is in the window, not in the live store and stale on disk.
func (x *Indexer) backfillProject(ctx context.Context, cutoff time.Time, projectDir string) {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		isTranscript := !entry.IsDir() && strings.HasSuffix(entry.Name(), transcriptSuffix)
		if !isTranscript {
			continue
		}

		info, err := entry.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			continue
		}
		x.backfillTranscript(filepath.Join(projectDir, entry.Name()), info)
	}
}

func (x *Indexer) backfillTranscript(path string, info os.FileInfo) {
	id := session.Id(strings.TrimSuffix(filepath.Base(path), transcriptSuffix))
	if _, isLive := x.store.GetById(id); isLive {
		return
	}

	source := Source{ModTime: info.ModTime(), Size: info.Size()}
	if x.isCurrent(id, source) {
		return
	}

	loaded, err := x.loader.LoadTree(session.AgentClaude, backfillDepth, path)
	if err != nil {
		slog.Debug("Indexer.backfillTranscript: Skipped", "path", path, "err", err)
		return
	}
	x.write(NewRecord(loaded, source))
}

// isCurrent reports whether the stored record was built from this exact transcript state.
func (x *Indexer) isCurrent(id session.Id, source Source) bool {
	data, ok := x.stateDir.ReadSessionMeta(string(session.AgentClaude), string(id))
	if !ok {
		return false
	}

	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return false
	}
	return record.Source.Size == source.Size && record.Source.ModTime.Equal(source.ModTime)
}

func (x *Indexer) write(record *Record) {
	data, err := json.Marshal(record)
	if err != nil {
		return
	}

	err = x.stateDir.WriteSessionMeta(string(session.AgentClaude), string(record.SessionId), data)
	if err != nil {
		slog.Warn("Indexer.write: Failed to write session meta", "session", record.SessionId, "err", err)
	}
}

// Backfill records the window's transcripts the live store does not hold; one instance on the machine runs it at a time, the rest skip.
func (x *Indexer) Backfill(ctx context.Context) {
	if x.stateDir == nil {
		return
	}

	release, isAcquired := x.stateDir.AcquireIndexLock(backfillLockStale)
	if !isAcquired {
		return
	}
	defer release()

	startedAt := time.Now()
	cutoff := startedAt.Add(-Window)
	projectDirs, err := os.ReadDir(x.projectsDir)
	if err != nil {
		return
	}
	for _, projectDir := range projectDirs {
		if !projectDir.IsDir() {
			continue
		}
		x.backfillProject(ctx, cutoff, filepath.Join(x.projectsDir, projectDir.Name()))
	}
	slog.Info("Indexer.Backfill: Done", "took", time.Since(startedAt).Round(time.Millisecond))
}

// PersistLive writes the record of every live Claude session that changed since this instance last wrote it.
func (x *Indexer) PersistLive() {
	if x.stateDir == nil {
		return
	}

	records := make([]*Record, 0)
	x.store.WithSessions([]session.Agent{session.AgentClaude}, func(sessions []*session.Session) {
		x.mu.Lock()
		defer x.mu.Unlock()

		for _, currentSession := range sessions {
			id := currentSession.Meta.SessionId
			if x.persistedUntil[id].Equal(currentSession.LastActive) {
				continue
			}

			x.persistedUntil[id] = currentSession.LastActive
			records = append(records, NewRecord(currentSession, sourceOf(currentSession.FilePath)))
		}
	})

	for _, record := range records {
		x.write(record)
	}
}

// Records reads every stored session summary; a record that does not decode is skipped.
func (x *Indexer) Records() []*Record {
	if x.stateDir == nil {
		return nil
	}

	all := x.stateDir.ReadSessionMetas(string(session.AgentClaude))
	records := make([]*Record, 0, len(all))
	for _, data := range all {
		var record Record
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}
		records = append(records, &record)
	}
	return records
}

// SessionRun pairs a run with the session it ran in.
type SessionRun struct {
	Record *Record
	Run    *Run
}

// RunsOf lists the skill's runs started within the window, newest first; a plugin-namespaced invocation matches too.
func RunsOf(records []*Record, skill string, now time.Time) []*SessionRun {
	cutoff := now.Add(-Window)
	runs := make([]*SessionRun, 0)
	for _, record := range records {
		for _, run := range record.Runs {
			isInWindow := !run.StartedAt.Before(cutoff)
			if isInWindow && isSkillMatch(run.Skill, skill) {
				runs = append(runs, &SessionRun{Record: record, Run: run})
			}
		}
	}
	slices.SortFunc(runs, compareNewestFirst)
	return runs
}

func compareNewestFirst(left, right *SessionRun) int {
	return cmp.Compare(right.Run.StartedAt.UnixNano(), left.Run.StartedAt.UnixNano())
}

// skillNamespaceSeparator joins a plugin's name and its skill's name, as in "plugin:skill".
const skillNamespaceSeparator = ":"

func isSkillMatch(invoked, wanted string) bool {
	if invoked == wanted {
		return true
	}
	return strings.HasSuffix(invoked, skillNamespaceSeparator+wanted)
}

func sourceOf(path string) Source {
	info, err := os.Stat(path)
	if err != nil {
		return Source{}
	}
	return Source{ModTime: info.ModTime(), Size: info.Size()}
}
