package store

import (
	"sort"
	"sync"

	"github.com/kevinhorst/peek-mcp/models"
)

type Store struct {
	mu       sync.RWMutex
	sessions map[models.SessionID]*models.Session
	depth    int
}

func New(depth int) *Store {
	return &Store{
		sessions: make(map[models.SessionID]*models.Session),
		depth:    depth,
	}
}

func (s *Store) GetOrCreate(id, source string) *models.Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID := models.SessionID(id)
	sessionSource := models.SessionSource(source)

	if session, ok := s.sessions[sessionID]; ok {
		return session
	}

	session := &models.Session{
		Info: &models.SessionInfo{
			ID:     sessionID,
			Source: sessionSource,
		},
		Turns: models.NewTurnBuffer(s.depth),
	}
	s.sessions[sessionID] = session
	return session
}

func (s *Store) Get(id string) (*models.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[models.SessionID(id)]
	return session, ok
}

func (s *Store) List() []models.SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]models.SessionInfo, 0, len(s.sessions))
	for _, session := range s.sessions {
		result = append(result, *session.Info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].LastActive.After(result[j].LastActive)
	})

	return result
}

func (s *Store) MostRecent() (*models.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var best *models.Session
	for _, session := range s.sessions {
		if best == nil || session.Info.LastActive.After(best.Info.LastActive) {
			best = session
		}
	}
	if best == nil {
		return nil, false
	}
	return best, true
}
