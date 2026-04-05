package watcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

const (
	claudeProjectsDir = "projects"
	codexSessionsDir  = "sessions"
	jsonlSuffix       = ".jsonl"
)

type Watcher struct {
	store      *store.Store
	claudeHome string
	codexHome  string

	mu    sync.Mutex
	files map[watchedFilePath]*watchedFile
}

type watchedFilePath string

type watchedFile struct {
	offset  int64
	partial []byte
	parser  lineParser
}

type lineParser interface {
	ParseLine(line []byte)
	Flush()
}

func New(s *store.Store, claudeHome, codexHome string) *Watcher {
	return &Watcher{
		store:      s,
		claudeHome: claudeHome,
		codexHome:  codexHome,
		files:      make(map[watchedFilePath]*watchedFile),
	}
}

func (w *Watcher) Run(ctx context.Context) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fsw.Close()

	// Add root directories and backfill existing files
	claudeProjects := filepath.Join(w.claudeHome, claudeProjectsDir)
	codexSessions := filepath.Join(w.codexHome, codexSessionsDir)

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

func (w *Watcher) walkAndWatch(watcher *fsnotify.Watcher, root string) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		log.Printf("watcher %s is not a directory", root)
		return
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			err = watcher.Add(path)
			if err != nil {
				return err
			}
			return nil
		}
		if strings.HasSuffix(path, jsonlSuffix) {
			w.readNewLines(path)
		}
		return nil
	})
	if err != nil {
		log.Printf("watcher error: %v", err)
	}
}

func (w *Watcher) handleEvent(watcher *fsnotify.Watcher, event fsnotify.Event) {
	if event.Has(fsnotify.Create) {
		info, err := os.Stat(event.Name)
		if err != nil {
			return
		}
		if info.IsDir() {
			err = watcher.Add(event.Name)
			if err != nil {
				fmt.Println(err)
			}
			return
		}
		if strings.HasSuffix(event.Name, jsonlSuffix) {
			w.readNewLines(event.Name)
		}
	}

	if event.Has(fsnotify.Write) && strings.HasSuffix(event.Name, jsonlSuffix) {
		w.readNewLines(event.Name)
	}
}

func (w *Watcher) readNewLines(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	fileInfo := w.getOrCreateFile(path)
	if fileInfo.offset > 0 {
		if _, err := file.Seek(fileInfo.offset, io.SeekStart); err != nil {
			return
		}
	}

	appended, err := io.ReadAll(file)
	if err != nil {
		return
	}

	fileInfo.offset += int64(len(appended))

	// Keep the unfinished trailing fragment, if any, and only send complete
	// newline-delimited records to the parser.
	buffer := make([]byte, 0, len(fileInfo.partial)+len(appended))
	buffer = append(buffer, fileInfo.partial...)
	buffer = append(buffer, appended...)

	fileInfo.partial = parseCompleteLines(buffer, fileInfo.parser)
	fileInfo.parser.Flush()
}

func parseCompleteLines(buffer []byte, parser lineParser) []byte {
	if len(buffer) == 0 {
		return nil
	}

	for _, part := range bytes.SplitAfter(buffer, []byte{'\n'}) {
		if len(part) == 0 {
			continue
		}
		// The last chunk may not have a newline yet, which means the writer has
		// not finished the JSONL record. Keep it for the next append.
		if part[len(part)-1] != '\n' {
			return bytes.Clone(part)
		}

		line := bytes.TrimSuffix(part, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) > 0 {
			parser.ParseLine(line)
		}
	}

	return nil
}

func (w *Watcher) getOrCreateFile(path string) *watchedFile {
	key := watchedFilePath(path)
	if file, ok := w.files[key]; ok {
		return file
	}

	file := &watchedFile{}
	if w.isClaude(path) {
		file.parser = parser.NewClaudeParser(w.store)
	} else {
		file.parser = parser.NewCodexParser(w.store)
	}
	w.files[key] = file
	return file
}

func (w *Watcher) isClaude(path string) bool {
	claudeProjects := filepath.Join(w.claudeHome, claudeProjectsDir)
	return strings.HasPrefix(path, claudeProjects)
}
