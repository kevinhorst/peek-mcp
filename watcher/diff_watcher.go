package watcher

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/kevinhorst/peek-mcp/session"
)

type DiffWatcher struct {
	store   *session.Store
	target  string
	running sync.Map
}

func NewDiffWatcher(store *session.Store, target string) *DiffWatcher {
	return &DiffWatcher{store: store, target: target}
}

func (w *DiffWatcher) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case id := <-w.store.TurnAdded:
			sess, ok := w.store.GetById(id)
			if !ok || sess.Meta.CWD == "" {
				continue
			}
			if _, loaded := w.running.LoadOrStore(id, struct{}{}); loaded {
				continue
			}
			go w.refresh(ctx, id, sess.Meta.CWD)
		}
	}
}

func (w *DiffWatcher) refresh(ctx context.Context, id session.Id, cwd string) {
	defer w.running.Delete(id)

	if _, err := os.Stat(cwd); err != nil {
		return
	}

	cmd := exec.CommandContext(ctx, "git", "diff", w.target)
	cmd.Dir = cwd
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			log.Printf("DiffWatcher: git diff failed for session %s: %s", id, exitErr.Stderr)
		} else {
			log.Printf("DiffWatcher: git diff failed for session %s: %v", id, err)
		}
		return
	}
	w.store.UpdateDiff(id, w.target, string(output))
	log.Printf("DiffWatcher: refreshed diff for session %s against %s (%d bytes)", id, w.target, len(output))
}
