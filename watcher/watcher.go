package watcher

import (
	"bufio"
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/kevinhorst/peek-mcp/parser"
	"github.com/kevinhorst/peek-mcp/store"
)

type Watcher struct {
	store      *store.Store
	claudeHome string
	codexHome  string

	mu      sync.Mutex
	offsets map[string]int64
	parsers map[string]lineParser
}

type lineParser interface {
	ParseLine(line []byte)
}

func New(s *store.Store, claudeHome, codexHome string) *Watcher {
	return &Watcher{
		store:      s,
		claudeHome: claudeHome,
		codexHome:  codexHome,
		offsets:    make(map[string]int64),
		parsers:    make(map[string]lineParser),
	}
}

func (w *Watcher) Run(ctx context.Context) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fsw.Close()

	// Add root directories and backfill existing files
	claudeProjects := filepath.Join(w.claudeHome, "projects")
	codexSessions := filepath.Join(w.codexHome, "sessions")

	w.walkAndWatch(fsw, claudeProjects)
	w.walkAndWatch(fsw, codexSessions)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-fsw.Events:
			if !ok {
				return nil
			}
			w.handleEvent(fsw, event)
		case err, ok := <-fsw.Errors:
			if !ok {
				return nil
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

func (w *Watcher) walkAndWatch(fsw *fsnotify.Watcher, root string) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return
	}

	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			fsw.Add(path)
			return nil
		}
		if strings.HasSuffix(path, ".jsonl") {
			w.readNewLines(path)
		}
		return nil
	})
}

func (w *Watcher) handleEvent(fsw *fsnotify.Watcher, event fsnotify.Event) {
	if event.Has(fsnotify.Create) {
		info, err := os.Stat(event.Name)
		if err != nil {
			return
		}
		if info.IsDir() {
			fsw.Add(event.Name)
			return
		}
		if strings.HasSuffix(event.Name, ".jsonl") {
			w.readNewLines(event.Name)
		}
	}

	if event.Has(fsnotify.Write) && strings.HasSuffix(event.Name, ".jsonl") {
		w.readNewLines(event.Name)
	}
}

func (w *Watcher) readNewLines(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	offset := w.offsets[path]
	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return
		}
	}

	p := w.getOrCreateParser(path)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		p.ParseLine(line)
	}

	newOffset, _ := f.Seek(0, 1) // current position
	w.offsets[path] = newOffset

	// Flush Claude parsers to commit any pending assistant turns
	if cp, ok := p.(*parser.ClaudeParser); ok {
		cp.Flush()
	}
}

func (w *Watcher) getOrCreateParser(path string) lineParser {
	if p, ok := w.parsers[path]; ok {
		return p
	}

	var p lineParser
	if w.isClaude(path) {
		p = parser.NewClaudeParser(w.store)
	} else {
		p = parser.NewCodexParser(w.store)
	}
	w.parsers[path] = p
	return p
}

func (w *Watcher) isClaude(path string) bool {
	claudeProjects := filepath.Join(w.claudeHome, "projects")
	return strings.HasPrefix(path, claudeProjects)
}
