package telemetry

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/kevinhorst/peek-mcp/state"
)

const (
	maxSessions = 1000
	agentClaude = "claude"
)

type SessionStats struct {
	ActiveSeconds float64
	CostUSD       float64
	UpdatedAt     time.Time
}

type Store struct {
	mu       sync.RWMutex
	now      func() time.Time
	sessions map[string]*SessionStats
	StateDir *state.Dir
}

type persistedStats struct {
	ActiveSeconds float64   `json:"active_seconds"`
	CostUSD       float64   `json:"cost_usd"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func NewStore() *Store {
	return &Store{
		now:      time.Now,
		sessions: make(map[string]*SessionStats),
	}
}

func (s *Store) Get(sessionId string) (SessionStats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats, ok := s.sessions[sessionId]
	if !ok {
		return SessionStats{}, false
	}
	return *stats, true
}

func (s *Store) fold(sessionId, metricName string, value float64, isDelta bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats, ok := s.sessions[sessionId]
	if !ok {
		if len(s.sessions) >= maxSessions {
			s.evictOldest()
		}
		stats = &SessionStats{}
		s.sessions[sessionId] = stats
	}
	stats.UpdatedAt = s.now()

	switch metricName {
	case metricActiveTime:
		stats.ActiveSeconds = foldValue(stats.ActiveSeconds, value, isDelta)
	case metricCostUsage:
		stats.CostUSD = foldValue(stats.CostUSD, value, isDelta)
	}
	s.persist(sessionId, stats)
}

func (s *Store) persist(sessionId string, stats *SessionStats) {
	if s.StateDir == nil {
		return
	}

	data, err := json.Marshal(persistedStats{
		ActiveSeconds: stats.ActiveSeconds,
		CostUSD:       stats.CostUSD,
		UpdatedAt:     stats.UpdatedAt,
	})
	if err != nil {
		return
	}
	_ = s.StateDir.WriteTelemetry(agentClaude, sessionId, string(data))
}

func ReadPersisted(dir *state.Dir, sessionId string) (SessionStats, bool) {
	if dir == nil {
		return SessionStats{}, false
	}

	content, err := dir.ReadTelemetry(agentClaude, sessionId)
	if err != nil {
		return SessionStats{}, false
	}

	var stored persistedStats
	if err := json.Unmarshal([]byte(content), &stored); err != nil {
		return SessionStats{}, false
	}
	return SessionStats{ActiveSeconds: stored.ActiveSeconds, CostUSD: stored.CostUSD, UpdatedAt: stored.UpdatedAt}, true
}

func (s *Store) evictOldest() {
	var oldestId string
	var oldest time.Time
	for id, stats := range s.sessions {
		if oldestId == "" || stats.UpdatedAt.Before(oldest) {
			oldestId = id
			oldest = stats.UpdatedAt
		}
	}
	delete(s.sessions, oldestId)
}

func foldValue(current, incoming float64, isDelta bool) float64 {
	if isDelta {
		return current + incoming
	}
	return max(current, incoming)
}
