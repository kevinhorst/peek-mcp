package session

import (
	"maps"
	"os"
	"slices"
	"sort"
	"sync"
)

type Store struct {
	mu        sync.RWMutex
	sessions  map[Id]*Session
	depth     int
	TurnAdded chan Id
}

func NewStore(depth int) *Store {
	return &Store{
		sessions:  make(map[Id]*Session),
		depth:     depth,
		TurnAdded: make(chan Id, 16), // small fixed buffer; dropped notifications are fine — next turn re-triggers
	}
}

func (s *Store) AddTurnBySessionId(id Id, source Source, turn *Turn) {
	session := s.getOrCreate(id, source)

	if turn.PlanFilePath != "" {
		content, _ := os.ReadFile(turn.PlanFilePath)
		s.mu.Lock()
		session.PlanFilePath = turn.PlanFilePath
		session.PlanContent = string(content)
		s.mu.Unlock()
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

	if session, ok := s.sessions[id]; ok {
		session.DiffOutput = output
		session.DiffTarget = target
	}
}

func (s *Store) UpdatePlanForPath(filePath, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

func (s *Store) Last() (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := s.sortByLastActiveDesc()
	if len(sessions) == 0 {
		return nil, false
	}

	return sessions[0], true
}

func (s *Store) List() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.sortByLastActiveDesc()
}

func (s *Store) getOrCreate(id Id, source Source) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[id]; ok {
		return session
	}

	session := &Session{
		Meta:          Meta{SessionId: id},
		Source:        source,
		TurnsFinished: NewTurnBuffer(s.depth),
	}
	s.sessions[id] = session
	return session
}

func (s *Store) sortByLastActiveDesc() []*Session {
	sessions := slices.Collect(maps.Values(s.sessions))
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[j].LastActive.Before(sessions[i].LastActive)
	})

	return sessions
}
