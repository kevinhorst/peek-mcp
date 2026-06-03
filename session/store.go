package session

import (
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"sort"
	"sync"
)

type Store struct {
	mu            sync.RWMutex
	sessions      map[Id]*Session
	depth         int
	enabledAgents []Agent
	TurnAdded     chan Id
}

func NewStore(depth int, agents ...Agent) *Store {
	return &Store{
		sessions:      make(map[Id]*Session),
		depth:         depth,
		enabledAgents: agents,
		TurnAdded:     make(chan Id, 16), // small fixed buffer; dropped notifications are fine — next turn re-triggers
	}
}

// TODO: Agent.IsValid
func (s *Store) ResolveAgent(agent Agent) (Agent, error) {
	if agent != "" {
		if agent != AgentClaude && agent != AgentCodex {
			return "", fmt.Errorf("agent must be \"claude\" or \"codex\", got %q", agent)
		}
		return agent, nil
	}
	if len(s.enabledAgents) == 1 {
		return s.enabledAgents[0], nil
	}
	return "", fmt.Errorf("agent parameter is required (\"claude\" or \"codex\")")
}

func (s *Store) AddTurnBySessionId(id Id, agent Agent, turn *Turn) {
	session := s.getOrCreate(id, agent)
	s.mu.Lock()
	defer s.mu.Unlock()

	// update plan content only
	if turn.PlanFilePath != "" {
		slog.Debug("Updating plan", "session", id)
		session.PlanFilePath = turn.PlanFilePath

		if turn.PlanContent != "" {
			session.PlanContent = turn.PlanContent
			return
		}

		content, err := os.ReadFile(turn.PlanFilePath)
		if err != nil {
			slog.Warn("Failed to read plan file", "path", turn.PlanFilePath, "err", err)
			return
		}
		session.PlanContent = string(content)
		return
	}

	session.AddTurn(turn)

	select {
	case s.TurnAdded <- id:
	default:
	}
}

func (s *Store) UpdateDiff(id Id, target, output string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slog.Debug("Updating diff", "session", id)

	if session, ok := s.sessions[id]; ok {
		session.DiffOutput = output
		session.DiffTarget = target
	}
}

func (s *Store) UpdateUncommittedDiff(id Id, output string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.sessions[id]; ok {
		session.UncommittedDiff = output
	}
}

func (s *Store) UpdatePlanForPath(filePath, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slog.Debug("Updating plan", "path", filePath)

	for _, session := range s.sessions {
		if session.PlanFilePath == filePath {
			session.PlanContent = content
		}
	}
}

func (s *Store) GetById(id Id) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, false
	}

	return session, ok
}

func (s *Store) Last(agents ...Agent) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := s.sortByLastActiveDesc(agents...)
	if len(sessions) == 0 {
		return nil, false
	}

	return sessions[0], true
}

func (s *Store) List(agents ...Agent) []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.sortByLastActiveDesc(agents...)
}

func (s *Store) getOrCreate(id Id, agent Agent) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[id]; ok {
		return session
	}

	session := &Session{
		Meta:          Meta{SessionId: id},
		Agent:         agent,
		TurnsFinished: NewTurnBuffer(s.depth),
	}
	s.sessions[id] = session
	return session
}

func (s *Store) sortByLastActiveDesc(agents ...Agent) []*Session {
	sessions := slices.Collect(maps.Values(s.sessions))
	//TODO: Filter by agent
	if len(agents) > 0 {
		agent := agents[0]
		sessions = slices.DeleteFunc(sessions, func(sess *Session) bool {
			return sess.Agent != agent
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[j].LastActive.Before(sessions[i].LastActive)
	})

	return sessions
}
