package watcher

import (
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

// Load reads the transcripts in order into a throwaway store keeping depth turns per scope; the parent transcript goes first.
func (l *TurnLoader) Load(agent session.Agent, depth int, id session.Id, paths ...string) (*session.Session, error) {
	newParser, ok := l.parserByAgent[agent]
	if !ok {
		return nil, errors.Errorf("TurnLoader.Load: No parser for agent %s", agent)
	}

	store := session.NewStore(depth, events.NewBroker(), agent)
	reader := New(agent, "", 0, newParser, store)
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
