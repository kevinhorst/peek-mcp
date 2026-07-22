package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/pkg/errors"
)

const (
	jsonlSuffix = ".jsonl"
)

const (
	agentFilePrefix  = "agent-"
	metaJsonSuffix   = ".meta.json"
	subagentsDirName = "subagents"
)

type watchedFile struct {
	offset int64
	parser Parser
}
type Watcher struct {
	agent     session.Agent
	agentDir  string
	files     map[string]*watchedFile
	mu        sync.Mutex
	newParser func() Parser
	store     *session.Store
}

func New(agent session.Agent, agentDir string, newParser func() Parser, store *session.Store) *Watcher {
	return &Watcher{
		agent:     agent,
		agentDir:  agentDir,
		newParser: newParser,
		store:     store,
		files:     make(map[string]*watchedFile),
	}
}

func (w *Watcher) Run(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Add root directories and backfill existing files
	w.walkAndWatch(watcher, w.agentDir)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				slog.Info("watcher closed")
				return nil
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}

			// new directory, new session has been started
			path := event.Name
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				if !event.Has(fsnotify.Create) {
					continue
				}

				w.walkAndWatch(watcher, path)
				continue
			}

			// new or changed file
			if strings.HasSuffix(path, jsonlSuffix) {
				err = w.readNewLines(path)
				if err != nil {
					slog.Warn("readNewLines", "err", err)
				}
			}

			if isSubagentMetaPath(path) {
				w.readSubagentMeta(path)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Error("watcher error", "err", err)
		}
	}
}

func (w *Watcher) walkAndWatch(watcher *fsnotify.Watcher, root string) {
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		slog.Warn("walkAndWatch: not a directory", "path", root)
		return
	}

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {

			err = watcher.Add(path)
			if err != nil {
				return err
			}
			return nil
		}
		if strings.HasSuffix(path, jsonlSuffix) {
			err = w.readNewLines(path)
			if err != nil {
				slog.Warn("walkAndWatch: readNewLines", "err", err)
			}
		}
		if isSubagentMetaPath(path) {
			w.readSubagentMeta(path)
		}
		return nil
	})
	if err != nil {
		slog.Error("walkAndWatch error", "err", err)
	}
}

func (w *Watcher) readNewLines(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	file, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "Watcher.readNewLines")
	}
	defer file.Close()

	watched, ok := w.files[path]
	if !ok {
		watched = &watchedFile{parser: w.newParser()}
	}

	if watched.offset > 0 {
		if _, err := file.Seek(watched.offset, io.SeekStart); err != nil {
			return errors.Wrapf(err, "Watcher.readNewLines")
		}
	}

	newLines, err := io.ReadAll(file)
	if err != nil {
		return errors.Wrapf(err, "Watcher.readNewLines")
	}

	// only count bytes from complete lines
	var consumed int64

	for _, part := range bytes.SplitAfter(newLines, []byte{'\n'}) {
		if len(part) == 0 {
			continue
		}
		if part[len(part)-1] != '\n' {
			break // incomplete line, stop — we'll re-read it next time
		}

		consumed += int64(len(part))

		line := bytes.TrimSuffix(part, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) > 0 {
			turn := watched.parser.ParseLine(line)
			err = turn.Validate()
			if err != nil {
				continue
			}
			turn.FilePath = path

			w.store.AddTurnBySessionId(turn.Meta.SessionId, w.agent, turn)
		}
	}

	watched.offset += consumed
	w.files[path] = watched
	return nil
}

type subagentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
	SpawnDepth  int    `json:"spawnDepth"`
	ToolUseId   string `json:"toolUseId"`
}

func isSubagentMetaPath(path string) bool {
	if !strings.HasSuffix(path, metaJsonSuffix) {
		return false
	}
	if !strings.HasPrefix(filepath.Base(path), agentFilePrefix) {
		return false
	}
	return filepath.Base(filepath.Dir(path)) == subagentsDirName
}

// The meta file carries no session id — the parent session id is the
// grandparent directory name (<project>/<sessionId>/subagents/), the one
// path-derived identity in the codebase. The files map marks it processed
// so re-walks do not emit duplicate spawned events.
func (w *Watcher) readSubagentMeta(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, done := w.files[path]; done {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("Watcher.readSubagentMeta: Failed to read meta file", "path", path, "err", err)
		return
	}

	var meta subagentMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		slog.Warn("Watcher.readSubagentMeta: Failed to parse meta file", "path", path, "err", err)
		return
	}

	w.files[path] = &watchedFile{offset: info.Size()}

	agentId := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), agentFilePrefix), metaJsonSuffix)
	sessionId := session.Id(filepath.Base(filepath.Dir(filepath.Dir(path))))

	payload := &session.SubagentPayload{
		AgentId:     agentId,
		AgentType:   meta.AgentType,
		Description: meta.Description,
		SpawnDepth:  meta.SpawnDepth,
		ToolUseId:   meta.ToolUseId,
	}
	event := &session.Event{
		Actor:     agentId,
		Kind:      session.EventKindSubagentSpawned,
		Subagent:  payload,
		Timestamp: info.ModTime(),
	}
	turn := &session.Turn{
		Events: []*session.Event{event},
		Meta:   &session.Meta{SessionId: sessionId},
	}

	w.store.AddTurnBySessionId(sessionId, w.agent, turn)
}
