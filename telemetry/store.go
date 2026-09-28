package telemetry

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/kevinhorst/peek-mcp/state"
)

const (
	maxSessions           = 1000
	maxPermissionRequests = 2000
	maxPendingCommands    = 256
	agentClaude           = "claude"
)

type PermissionDecision struct {
	Command   string    `json:"command,omitempty"`
	Decision  string    `json:"decision"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	Tool      string    `json:"tool"`
	ToolUseId string    `json:"tool_use_id,omitempty"`
}

type PermissionStats struct {
	Aborted        int                  `json:"aborted,omitempty"`
	AutoAllowed    int                  `json:"auto_allowed,omitempty"`
	ConfigDenied   int                  `json:"config_denied,omitempty"`
	HookAllowed    int                  `json:"hook_allowed,omitempty"`
	HookDenied     int                  `json:"hook_denied,omitempty"`
	PromptedAlways int                  `json:"prompted_always,omitempty"`
	PromptedOnce   int                  `json:"prompted_once,omitempty"`
	Rejected       int                  `json:"rejected,omitempty"`
	Requests       []PermissionDecision `json:"requests,omitempty"`
}

// count splits config and hook decisions by their verdict, so a config-level
// denial is never read as an allow.
func (p *PermissionStats) count(decision *PermissionDecision) {
	isReject := decision.Decision == decisionReject
	switch decision.Source {
	case sourceConfig:
		if isReject {
			p.ConfigDenied++
			return
		}
		p.AutoAllowed++
	case sourceHook:
		if isReject {
			p.HookDenied++
			return
		}
		p.HookAllowed++
	case sourceUserTemporary:
		p.PromptedOnce++
	case sourceUserPermanent:
		p.PromptedAlways++
	case sourceUserReject:
		p.Rejected++
	case sourceUserAbort:
		p.Aborted++
	}
}

func (p *PermissionStats) IsZero() bool {
	hasNoAllows := p.AutoAllowed == 0 && p.HookAllowed == 0 && p.PromptedOnce == 0 && p.PromptedAlways == 0
	hasNoDenials := p.ConfigDenied == 0 && p.HookDenied == 0 && p.Rejected == 0 && p.Aborted == 0
	return hasNoAllows && hasNoDenials && len(p.Requests) == 0
}

type SessionStats struct {
	ActiveSeconds float64
	CostUSD       float64
	Permissions   PermissionStats
	UpdatedAt     time.Time
}

// pendingCommand points at a Requests entry awaiting its command from the
// matching tool_result event.
type pendingCommand struct {
	sessionId string
	index     int
}

type Store struct {
	mu              sync.RWMutex
	now             func() time.Time
	sessions        map[string]*SessionStats
	pendingCommands map[string]pendingCommand
	StateDir        *state.Dir
}

type persistedStats struct {
	ActiveSeconds float64         `json:"active_seconds"`
	CostUSD       float64         `json:"cost_usd"`
	Permissions   PermissionStats `json:"permissions"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

func NewStore() *Store {
	return &Store{
		now:             time.Now,
		sessions:        make(map[string]*SessionStats),
		pendingCommands: make(map[string]pendingCommand),
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

func (s *Store) statsFor(sessionId string) *SessionStats {
	stats, ok := s.sessions[sessionId]
	if !ok {
		if len(s.sessions) >= maxSessions {
			s.evictOldest()
		}
		stats = s.restoredStats(sessionId)
		s.sessions[sessionId] = stats
	}
	stats.UpdatedAt = s.now()
	return stats
}

func (s *Store) fold(sessionId, metricName string, value float64, isDelta bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := s.statsFor(sessionId)

	switch metricName {
	case metricActiveTime:
		stats.ActiveSeconds = foldValue(stats.ActiveSeconds, value, isDelta)
	case metricCostUsage:
		stats.CostUSD = foldValue(stats.CostUSD, value, isDelta)
	}
	s.persist(sessionId, stats)
}

func (s *Store) foldDecision(sessionId string, decision PermissionDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := s.statsFor(sessionId)
	stats.Permissions.count(&decision)

	hasRoom := len(stats.Permissions.Requests) < maxPermissionRequests
	if isListedDecision(&decision) && hasRoom {
		stats.Permissions.Requests = append(stats.Permissions.Requests, decision)
		if decision.ToolUseId != "" {
			if len(s.pendingCommands) >= maxPendingCommands {
				s.pendingCommands = make(map[string]pendingCommand)
			}
			s.pendingCommands[decision.ToolUseId] = pendingCommand{
				sessionId: sessionId,
				index:     len(stats.Permissions.Requests) - 1,
			}
		}
	}
	s.persist(sessionId, stats)
}

// enrichCommand fills the command of a prompted decision from the tool_result
// event sharing its tool_use_id; the decision event itself carries no input.
func (s *Store) enrichCommand(sessionId, toolUseId, command string) {
	if toolUseId == "" || command == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	pending, ok := s.pendingCommands[toolUseId]
	if !ok || pending.sessionId != sessionId {
		return
	}
	delete(s.pendingCommands, toolUseId)

	stats, ok := s.sessions[sessionId]
	if !ok || pending.index >= len(stats.Permissions.Requests) {
		return
	}
	stats.Permissions.Requests[pending.index].Command = command
	s.persist(sessionId, stats)
}

func (s *Store) persist(sessionId string, stats *SessionStats) {
	if s.StateDir == nil {
		return
	}

	data, err := json.Marshal(persistedStats{
		ActiveSeconds: stats.ActiveSeconds,
		CostUSD:       stats.CostUSD,
		Permissions:   stats.Permissions,
		UpdatedAt:     stats.UpdatedAt,
	})
	if err != nil {
		return
	}
	_ = s.StateDir.WriteTelemetry(agentClaude, sessionId, string(data))
}

// restoredStats continues a session peek holds no memory of (restart,
// eviction) from its persisted file instead of overwriting it with zero.
func (s *Store) restoredStats(sessionId string) *SessionStats {
	persisted, ok := ReadPersisted(s.StateDir, sessionId)
	if !ok {
		return &SessionStats{}
	}
	return &persisted
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
	return SessionStats{ActiveSeconds: stored.ActiveSeconds, CostUSD: stored.CostUSD, Permissions: stored.Permissions, UpdatedAt: stored.UpdatedAt}, true
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

// isListedDecision keeps the list to what an operator acts on: every denial
// and every prompted decision; config and hook allows are counted only.
func isListedDecision(decision *PermissionDecision) bool {
	if decision.Decision == decisionReject {
		return true
	}
	return decision.Source == sourceUserTemporary || decision.Source == sourceUserPermanent
}
