package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/kevinhorst/peek-mcp/session"
)

const mdSuffix = ".md"

type PlanWatcher struct {
	plansDir string
	store    *session.Store
}

func NewPlanWatcher(plansDir string, store *session.Store) *PlanWatcher {
	return &PlanWatcher{
		plansDir: plansDir,
		store:    store,
	}
}

func (w *PlanWatcher) Run(ctx context.Context) error {
	if err := os.MkdirAll(w.plansDir, 0755); err != nil {
		slog.Warn("PlanWatcher: could not create plans directory", "dir", w.plansDir, "err", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := watcher.Add(w.plansDir); err != nil {
		slog.Warn("PlanWatcher: could not watch", "dir", w.plansDir, "err", err)
		<-ctx.Done()
		return ctx.Err()
	}

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
			if !strings.HasSuffix(event.Name, mdSuffix) {
				continue
			}
			w.loadPlan(event.Name)
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Error("PlanWatcher error", "err", err)
		}
	}
}

func (w *PlanWatcher) loadPlan(path string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		slog.Warn("PlanWatcher.loadPlan: abs path", "err", err)
		return
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		slog.Warn("PlanWatcher.loadPlan: read file", "err", err)
		return
	}

	w.store.UpdatePlanForPath(absPath, string(content))
}
