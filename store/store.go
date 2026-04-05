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

	if sess, ok := s.sessions[sessionID]; ok {
		return sess
	}

	sess := &models.Session{
		Meta: &models.SessionMeta{
			ID:     sessionID,
			Source: sessionSource,
		},
		Turns: models.NewTurnBuffer(s.depth),
	}
	s.sessions[sessionID] = sess
	return sess
}

func (s *Store) Get(id string) (*models.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[models.SessionID(id)]
	return sess, ok
}

func (s *Store) List() []models.SessionMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]models.SessionMeta, 0, len(s.sessions))
	for _, sess := range s.sessions {
		result = append(result, *sess.Meta)
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
	for _, sess := range s.sessions {
		if best == nil || sess.Meta.LastActive.After(best.Meta.LastActive) {
			best = sess
		}
	}
	if best == nil {
		return nil, false
	}
	return best, true
}
