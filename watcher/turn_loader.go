package watcher

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
)

// TurnLoader re-parses a session's transcripts when a caller asks for more turns than the store's buffer keeps.
type TurnLoader struct {
	parserByAgent map[session.Agent]func() Parser
}

func NewTurnLoader(parserByAgent map[session.Agent]func() Parser) *TurnLoader {
	return &TurnLoader{parserByAgent: parserByAgent}
}

func (l *TurnLoader) newReader(agent session.Agent, depth int) (*Watcher, *session.Store, error) {
	newParser, ok := l.parserByAgent[agent]
	if !ok {
		return nil, nil, errors.Errorf("TurnLoader.newReader: No parser for agent %s", agent)
	}

	store := session.NewStore(depth, events.NewBroker(), agent)
	return New(agent, "", 0, newParser, store), store, nil
}

// Load reads the transcripts in order into a throwaway store keeping depth turns per scope; the parent transcript goes first.
func (l *TurnLoader) Load(agent session.Agent, depth int, id session.Id, paths ...string) (*session.Session, error) {
	reader, store, err := l.newReader(agent, depth)
	if err != nil {
		return nil, errors.Wrap(err, "TurnLoader.Load")
	}

	for _, path := range paths {
		err := reader.readNewLines(path)
		if err != nil {
			return nil, errors.Wrap(err, "TurnLoader.Load")
		}
	}

	loaded, found := store.GetById(id)
	if !found {
		return nil, errors.Errorf("TurnLoader.Load: Session %s not found in its transcripts", id)
	}
	return loaded, nil
}

// LoadTree reads a parent transcript and every file under its session's subagents dir into a throwaway store, agent types and journals included.
func (l *TurnLoader) LoadTree(agent session.Agent, depth int, parentPath string) (*session.Session, error) {
	reader, store, err := l.newReader(agent, depth)
	if err != nil {
		return nil, errors.Wrap(err, "TurnLoader.LoadTree")
	}

	err = reader.readNewLines(parentPath)
	if err != nil {
		return nil, errors.Wrap(err, "TurnLoader.LoadTree")
	}

	id := session.Id(strings.TrimSuffix(filepath.Base(parentPath), jsonlSuffix))
	subagentsDir := filepath.Join(strings.TrimSuffix(parentPath, jsonlSuffix), subagentsDirName)
	_ = filepath.WalkDir(subagentsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}

		reader.readSubagentFile(path)
		return nil
	})

	loaded, found := store.GetById(id)
	if !found {
		return nil, errors.Errorf("TurnLoader.LoadTree: Session %s not found in its transcript", id)
	}
	return loaded, nil
}
