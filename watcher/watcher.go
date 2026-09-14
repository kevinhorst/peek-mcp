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
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/pkg/errors"
)

const (
	jsonlSuffix = ".jsonl"
)

const eventChannelBuffer = 1024

type eventBatch struct {
	created []string
	dirty   map[string]struct{}
}

func newEventBatch() *eventBatch {
	return &eventBatch{dirty: make(map[string]struct{})}
}

func (b *eventBatch) add(event fsnotify.Event) {
	if event.Has(fsnotify.Create) {
		b.created = append(b.created, event.Name)
		return
	}
	if event.Has(fsnotify.Write) {
		b.dirty[event.Name] = struct{}{}
	}
}

func (b *eventBatch) drain(events <-chan fsnotify.Event) {
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			b.add(event)
		default:
			return
		}
	}
}

func isBeforeCutoff(entry fs.DirEntry, cutoff time.Time) bool {
	if cutoff.IsZero() {
		return false
	}

	info, err := entry.Info()
	if err != nil {
		return false
	}
	return info.ModTime().Before(cutoff)
}

const (
	agentFilePrefix  = "agent-"
	metaJsonSuffix   = ".meta.json"
	subagentsDirName = "subagents"
	journalFileName  = "journal.jsonl"
)

type subagentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
	SpawnDepth  int    `json:"spawnDepth"`
	ToolUseId   string `json:"toolUseId"`
}

type watchedFile struct {
	offset int64
	parser Parser
}
type Watcher struct {
	agent     session.Agent
	agentDir  string
	files     map[string]*watchedFile
	horizon   time.Duration
	mu        sync.Mutex
	newParser func() Parser
	store     *session.Store

	// Optional, set between New and Run.
	// TranscriptPathOk restricts which .jsonl files count as transcripts (nil = all).
	// Project labels every ingested turn's Meta.Project (empty = derive from CWD).
	TranscriptPathOk func(path string) bool
	Project          string
}

func New(agent session.Agent, agentDir string, horizon time.Duration, newParser func() Parser, store *session.Store) *Watcher {
	return &Watcher{
		agent:     agent,
		agentDir:  agentDir,
		files:     make(map[string]*watchedFile),
		horizon:   horizon,
		newParser: newParser,
		store:     store,
	}
}

func (w *Watcher) Run(ctx context.Context) error {
	watcher, err := fsnotify.NewBufferedWatcher(eventChannelBuffer)
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Add root directories and backfill existing files
	w.walkAndWatch(watcher, w.agentDir)
	w.store.SeedDiffCache()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				slog.Info("watcher closed")
				return nil
			}
			batch := newEventBatch()
			batch.add(event)
			batch.drain(watcher.Events)
			w.processBatch(watcher, batch)

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Error("watcher error", "err", err)
		}
	}
}

// processBatch handles a coalesced burst: creates first (a new directory must
// be watched before its files' writes are read), then each dirty path once.
func (w *Watcher) processBatch(watcher *fsnotify.Watcher, batch *eventBatch) {
	for _, path := range batch.created {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			w.walkAndWatch(watcher, path)
			continue
		}
		batch.dirty[path] = struct{}{}
	}

	for path := range batch.dirty {
		if isWorkflowJournalPath(path) {
			if err := w.readJournal(path); err != nil {
				slog.Warn("readJournal", "err", err)
			}
			continue
		}
		if w.isTranscriptPath(path) {
			if err := w.readNewLines(path); err != nil {
				slog.Warn("readNewLines", "err", err)
			}
		}
		if isSubagentMetaPath(path) {
			w.readSubagentMeta(path)
		}
	}
}

func (w *Watcher) walkAndWatch(watcher *fsnotify.Watcher, root string) {
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		slog.Warn("walkAndWatch: not a directory", "path", root)
		return
	}

	var subagentPaths []string
	cutoff := time.Time{}
	if w.horizon > 0 {
		cutoff = time.Now().Add(-w.horizon)
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
		if isBeforeCutoff(entry, cutoff) {
			return nil
		}
		if isSubagentPath(path) {
			subagentPaths = append(subagentPaths, path)
			return nil
		}
		if w.isTranscriptPath(path) {
			err = w.readNewLines(path)
			if err != nil {
				slog.Warn("walkAndWatch: readNewLines", "err", err)
			}
		}
		return nil
	})
	if err != nil {
		slog.Error("walkAndWatch error", "err", err)
	}

	for _, path := range subagentPaths {
		if isWorkflowJournalPath(path) {
			err = w.readJournal(path)
			if err != nil {
				slog.Warn("walkAndWatch: readJournal", "err", err)
			}
			continue
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
			w.stampProject(turn)

			w.store.AddTurnBySessionId(turn.Meta.SessionId, w.agent, turn)
		}
	}

	watched.offset += consumed
	w.files[path] = watched
	return nil
}

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

	root := subagentsRootDir(path)
	if root == "" {
		return
	}
	agentId := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), agentFilePrefix), metaJsonSuffix)
	sessionId := session.Id(filepath.Base(root))

	description := meta.Description
	if description == "" {
		description = workflowAgentLabel(path, agentId, root)
	}

	payload := &session.SubagentPayload{
		AgentId:     agentId,
		AgentType:   meta.AgentType,
		Description: description,
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
		Events:     []*session.Event{event},
		SubagentId: agentId,
		Meta:       &session.Meta{SessionId: sessionId},
	}

	w.store.AddTurnBySessionId(sessionId, w.agent, turn)
}

// maxJournalResultBytes mirrors claude.maxSubagentResultBytes.
const maxJournalResultBytes = 32 * 1024

type journalRecord struct {
	Type    string          `json:"type"`
	AgentId string          `json:"agentId"`
	Result  json.RawMessage `json:"result"`
}

func (w *Watcher) readJournal(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	root := subagentsRootDir(path)
	if root == "" {
		return nil
	}
	sessionId := session.Id(filepath.Base(root))

	file, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "Watcher.readJournal")
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return errors.Wrapf(err, "Watcher.readJournal")
	}
	modTime := info.ModTime()

	watched, ok := w.files[path]
	if !ok {
		watched = &watchedFile{}
	}

	if watched.offset > 0 {
		if _, err := file.Seek(watched.offset, io.SeekStart); err != nil {
			return errors.Wrapf(err, "Watcher.readJournal")
		}
	}

	newLines, err := io.ReadAll(file)
	if err != nil {
		return errors.Wrapf(err, "Watcher.readJournal")
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
		if len(line) == 0 {
			continue
		}

		var record journalRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.Type != "result" || record.AgentId == "" {
			continue
		}

		content := string(record.Result)
		if len(content) > maxJournalResultBytes {
			content = content[:maxJournalResultBytes]
		}

		event := &session.Event{
			Actor: record.AgentId,
			Kind:  session.EventKindSubagentResult,
			Subagent: &session.SubagentPayload{
				AgentId: record.AgentId,
				Content: content,
			},
			Timestamp: modTime,
		}
		turn := &session.Turn{
			Events:     []*session.Event{event},
			SubagentId: record.AgentId,
			Meta:       &session.Meta{SessionId: sessionId},
		}
		w.store.AddTurnBySessionId(sessionId, w.agent, turn)
	}

	watched.offset += consumed
	w.files[path] = watched
	return nil
}

func isWorkflowJournalPath(path string) bool {
	return filepath.Base(path) == journalFileName && subagentsRootDir(path) != ""
}

func (w *Watcher) isTranscriptPath(path string) bool {
	if !strings.HasSuffix(path, jsonlSuffix) {
		return false
	}
	if w.TranscriptPathOk != nil {
		return w.TranscriptPathOk(path)
	}
	return true
}

func (w *Watcher) stampProject(turn *session.Turn) {
	if turn.Meta == nil {
		return
	}
	if w.Project != "" {
		turn.Meta.Project = w.Project
		return
	}
	turn.Meta.Project = legacyProjectFromCwd(turn.Meta.CWD)
}

var userHome, _ = os.UserHomeDir()

// legacyProjectFromCwd maps the dead-generation desktop layout
// (cwd at or under ~/Claude_<Name>) to <Name>; everything else is "".
func legacyProjectFromCwd(cwd string) string {
	if userHome == "" || cwd == "" {
		return ""
	}
	prefix := strings.TrimSuffix(filepath.ToSlash(userHome), "/") + "/Claude_"
	slashCwd := filepath.ToSlash(cwd)
	if !strings.HasPrefix(slashCwd, prefix) {
		return ""
	}
	rest := slashCwd[len(prefix):]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

func isSubagentMetaPath(path string) bool {
	if !strings.HasSuffix(path, metaJsonSuffix) {
		return false
	}
	if !strings.HasPrefix(filepath.Base(path), agentFilePrefix) {
		return false
	}
	return subagentsRootDir(path) != ""
}

func isSubagentPath(path string) bool {
	return subagentsRootDir(path) != ""
}

func subagentsRootDir(path string) string {
	dir := filepath.Dir(path)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		if filepath.Base(dir) == subagentsDirName {
			return parent
		}
		dir = parent
	}
}

type workflowManifest struct {
	WorkflowProgress []struct {
		AgentId string `json:"agentId"`
		Label   string `json:"label"`
	} `json:"workflowProgress"`
}

func workflowAgentLabel(metaPath, agentId, root string) string {
	runDir := filepath.Base(filepath.Dir(metaPath))
	if !strings.HasPrefix(runDir, "wf_") {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(root, "workflows", runDir+".json"))
	if err != nil {
		return ""
	}
	var manifest workflowManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ""
	}
	for _, agent := range manifest.WorkflowProgress {
		if agent.AgentId == agentId {
			return agent.Label
		}
	}
	return ""
}
