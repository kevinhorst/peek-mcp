package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

type Store struct {
	mu sync.RWMutex

	IdByTitle     map[string]Id // SHA-256 hex of normalized title → session Id
	TurnAdded     chan Id
	depth         int
	enabledAgents []Agent
	sessions      map[Id]*Session
}

func NewStore(depth int, agents ...Agent) *Store {
	return &Store{
		sessions:      make(map[Id]*Session),
		IdByTitle:     make(map[string]Id),
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

	// update only title
	if session.HasNewTitle(turn.CustomTitle) {
		slog.Debug("Updating title", "session", id, "title", turn.CustomTitle)

		old := hashTitle(session.Title)
		if session.Title != "" {
			delete(s.IdByTitle, old)
		}

		session.Title = turn.CustomTitle
		s.IdByTitle[hashTitle(turn.CustomTitle)] = id
		return
	}

	// update only plan content
	if turn.PlanFilePath != "" {
		slog.Debug("Updating plan", "session", id)
		session.PlanFilePath = turn.PlanFilePath

		if turn.PlanContent != "" {
			session.PlanContent = turn.PlanContent
			return
		}

		if content, err := os.ReadFile(turn.PlanFilePath); err == nil {
			session.PlanContent = string(content)
			return
		}

		// Worktree fallback: Claude Code reports plan path as ~/.claude/plans/<name>
		// but worktree sessions write to <cwd>/.claude/plans/<name>.
		if cwd := turn.Meta.CWD; cwd != "" {
			alt := filepath.Join(cwd, ".claude", "plans", filepath.Base(turn.PlanFilePath))
			if content, err := os.ReadFile(alt); err == nil {
				session.PlanFilePath = alt
				session.PlanContent = string(content)
				return
			}
		}

		slog.Warn("Failed to read plan file", "path", turn.PlanFilePath)
		return
	}

	// update user or assistent turn
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

func (s *Store) GetByTitle(title string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.IdByTitle[hashTitle(title)]
	if !ok {
		return nil, false
	}
	sess, ok := s.sessions[id]
	return sess, ok
}

// hashTitle returns the SHA-256 hex digest of the normalized (lowercase, trimmed) title.
func hashTitle(title string) string {
	if title == "" {
		return ""
	}

	normalized := strings.ToLower(strings.TrimSpace(title))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
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
