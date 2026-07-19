package watcher

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kevinhorst/peek-mcp/codex"
	"github.com/kevinhorst/peek-mcp/session"
)

const (
	indexDebounce      = 250 * time.Millisecond
	indexWarnSizeBytes = 1 << 20
)

type CodexIndexWatcher struct {
	codexHome string
	store     *session.Store
}

func NewCodexIndexWatcher(codexHome string, store *session.Store) *CodexIndexWatcher {
	return &CodexIndexWatcher{
		codexHome: codexHome,
		store:     store,
	}
}

func (w *CodexIndexWatcher) loadIndex() {
	indexPath := filepath.Join(w.codexHome, codex.IndexFile)
	content, err := os.ReadFile(indexPath)
	if err != nil {
		slog.Debug("CodexIndexWatcher.loadIndex: Failed to read index", "err", err)
		return
	}

	if len(content) > indexWarnSizeBytes {
		slog.Warn("CodexIndexWatcher.loadIndex: Index file unexpectedly large", "bytes", len(content))
	}

	entries := make(map[session.Id]*codex.IndexEntry)
	for line := range strings.Lines(string(content)) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		var entry codex.IndexEntry
		if err := json.Unmarshal([]byte(trimmed), &entry); err != nil {
			slog.Debug("CodexIndexWatcher.loadIndex: Skipping malformed line", "err", err)
			continue
		}
		if err := entry.Validate(); err != nil {
			slog.Debug("CodexIndexWatcher.loadIndex: Skipping invalid entry", "err", err)
			continue
		}

		entries[entry.Id] = &entry
	}

	for id, entry := range entries {
		turn := &session.Turn{
			CustomTitle: entry.ThreadName,
			Meta:        &session.Meta{SessionId: id},
			Timestamp:   entry.UpdatedAt,
			TitleSource: session.TitleSourceIndex,
		}
		w.store.AddTurnBySessionId(id, session.AgentCodex, turn)
	}
}

func (w *CodexIndexWatcher) Run(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := waitForDir(ctx, watcher, w.codexHome); err != nil {
		return err
	}

	w.loadIndex()

	debounce := time.NewTimer(indexDebounce)
	debounce.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}
			if filepath.Base(event.Name) != codex.IndexFile {
				continue
			}
			debounce.Reset(indexDebounce)
		case <-debounce.C:
			w.loadIndex()
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Error("CodexIndexWatcher error", "err", err)
		}
	}
}
