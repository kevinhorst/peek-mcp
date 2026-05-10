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
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/pkg/errors"
)

const (
	jsonlSuffix = ".jsonl"
)

type watchedFile struct {
	offset int64
}
type Watcher struct {
	agent    session.Source
	agentDir string
	files    map[string]*watchedFile
	mu       sync.Mutex
	parser   parser
	store    *session.Store
}

func New(agent session.Source, agentDir string, parser parser, store *session.Store) *Watcher {
	return &Watcher{
		agent:    agent,
		agentDir: agentDir,
		parser:   parser,
		store:    store,
		files:    make(map[string]*watchedFile),
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
				log.Println("watcher closed")
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

				err = watcher.Add(path)
				if err != nil {
					fmt.Println("Warning: watcher.Add:", err)
				}
				continue
			}

			// new or changed file
			if strings.HasSuffix(path, jsonlSuffix) {
				err = w.readNewLines(path)
				if err != nil {
					fmt.Println("Warning: w.readNewLines:", err)
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

func (w *Watcher) walkAndWatch(watcher *fsnotify.Watcher, root string) {
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		log.Printf("Watcher.walkAndWatch: watcher %s is not a directory", root)
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
				fmt.Println("Watcher.walkAndWatch: Warning", err)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("Watcher.walkAndWatch: watcher error: %v", err)
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
		watched = &watchedFile{}
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
			turn := w.parser.ParseLine(line)
			err = turn.Validate()
			if err != nil {
				continue
			}

			w.store.AddTurn(turn.Meta.SessionId, w.agent, turn)
		}
	}

	watched.offset += consumed
	w.files[path] = watched
	return nil
}
