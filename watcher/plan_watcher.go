package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := waitForDir(ctx, watcher, w.plansDir); err != nil {
		return err
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

func waitForDir(ctx context.Context, fsWatcher *fsnotify.Watcher, dir string) error {
	if err := fsWatcher.Add(dir); err == nil {
		return nil
	}

	slog.Info("waitForDir: Dir not found, waiting for creation", "dir", dir)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := fsWatcher.Add(dir); err == nil {
				slog.Info("waitForDir: Dir found, watching", "dir", dir)
				return nil
			}
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
