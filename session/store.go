package session

import (
	"maps"
	"slices"
	"sort"
	"sync"
)

type Store struct {
	mu       sync.RWMutex
	sessions map[Id]*Session
	depth    int
}

func New(depth int) *Store {
	return &Store{
		sessions: make(map[Id]*Session),
		depth:    depth,
	}
}

func (s *Store) GetOrCreate(id Id, source Source) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[id]; ok {
		return session
	}

	session := &Session{
		Info: &Info{
			Id:     id,
			Source: source,
		},
		Turns: NewTurnBuffer(s.depth),
	}
	s.sessions[id] = session
	return session
}

func (s *Store) Get(id Id) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, false
	}

	return session, ok
}

func (s *Store) List() []*Info {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sessions := s.SortByLastActiveAsc()

	result := make([]*Info, 0, len(s.sessions))
	for _, session := range sessions {
		result = append(result, session.Info)
	}

	return result
}

func (s *Store) Last() (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := s.SortByLastActiveAsc()
	if len(sessions) == 0 {
		return nil, false
	}

	return sessions[0], true
}

func (s *Store) SortByLastActiveAsc() []*Session {
	sessions := slices.Collect(maps.Values(s.sessions))
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Info.LastActive.Before(sessions[j].Info.LastActive)
	})

	return sessions
}
