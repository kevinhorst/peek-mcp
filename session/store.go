package session

import (
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	derivedTitleMaxRunes = 80
	maxTitleCandidates   = 5
)

type Store struct {
	mu sync.RWMutex

	TurnAdded      chan Id
	depth          int
	enabledAgents  []Agent
	plainTitleById map[Id]string
	sessions       map[Id]*Session
}

func NewStore(depth int, agents ...Agent) *Store {
	return &Store{
		sessions:       make(map[Id]*Session),
		plainTitleById: make(map[Id]string),
		depth:          depth,
		enabledAgents:  agents,
		TurnAdded:      make(chan Id, 16), // small fixed buffer; dropped notifications are fine — next turn re-triggers
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
	if turn.CustomTitle != "" {
		if session.LastActive.IsZero() && !turn.Timestamp.IsZero() {
			session.LastActive = turn.Timestamp
		}

		if !session.HasNewTitle(turn.CustomTitle, turn.TitleSource) {
			return
		}

		slog.Debug("Updating title", "session", id, "title", turn.CustomTitle, "source", turn.TitleSource)
		s.setTitle(session, turn.CustomTitle, turn.TitleSource)
		return
	}

	// update only plan content
	if turn.PlanFilePath != "" {
		slog.Debug("Updating plan", "session", id)
		session.PlanFilePath = turn.PlanFilePath
		s.updatePlanContent(session, turn)

		// Codex plan turns are also chat turns; Claude plan signals carry no text
		if turn.Text == "" {
			return
		}
	}

	isUntitled := session.Title == ""
	isUserPrompt := turn.Role == RoleUser && turn.Text != ""
	if isUntitled && isUserPrompt {
		if derivedTitle := deriveTitle(turn.Text); derivedTitle != "" {
			s.setTitle(session, derivedTitle, TitleSourceDerived)
		}
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

func (s *Store) GetByTitle(title string, agent Agent) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	needle := strings.ToLower(strings.TrimSpace(title))

	var exactMatches []*Session
	var substringMatches []*Session
	for id, plainTitle := range s.plainTitleById {
		sess, ok := s.sessions[id]
		if !ok {
			continue
		}
		if agent != "" && sess.Agent != agent {
			continue
		}
		if plainTitle == needle {
			exactMatches = append(exactMatches, sess)
			continue
		}
		if strings.Contains(plainTitle, needle) {
			substringMatches = append(substringMatches, sess)
		}
	}

	sortSessionsByLastActiveDesc(exactMatches)
	if len(exactMatches) > 0 {
		return exactMatches[0], nil
	}

	sortSessionsByLastActiveDesc(substringMatches)
	if len(substringMatches) == 1 {
		return substringMatches[0], nil
	}
	if len(substringMatches) == 0 {
		return nil, fmt.Errorf("no session matching title %q", title)
	}

	candidates := substringMatches
	if len(candidates) > maxTitleCandidates {
		candidates = candidates[:maxTitleCandidates]
	}

	lines := make([]string, 0, len(candidates))
	for _, sess := range candidates {
		line := fmt.Sprintf("%q (id %s, last active %s)", sess.Title, sess.Meta.SessionId, sess.LastActive.Format(time.RFC3339))
		lines = append(lines, line)
	}
	return nil, fmt.Errorf("multiple sessions match title %q: %s", title, strings.Join(lines, "; "))
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

func (s *Store) setTitle(session *Session, title string, source TitleSource) {
	session.Title = title
	session.TitleSource = source
	s.plainTitleById[session.Meta.SessionId] = strings.ToLower(strings.TrimSpace(title))
}

func (s *Store) updatePlanContent(session *Session, turn *Turn) {
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
}

func (s *Store) sortByLastActiveDesc(agents ...Agent) []*Session {
	sessions := slices.Collect(maps.Values(s.sessions))
	if len(agents) > 0 {
		agent := agents[0]
		sessions = slices.DeleteFunc(sessions, func(sess *Session) bool {
			return sess.Agent != agent
		})
	}

	sortSessionsByLastActiveDesc(sessions)
	return sessions
}

func deriveTitle(text string) string {
	firstLine := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if strings.HasPrefix(firstLine, "<") || strings.HasPrefix(firstLine, "# AGENTS.md") {
		return ""
	}

	runes := []rune(firstLine)
	if len(runes) > derivedTitleMaxRunes {
		return string(runes[:derivedTitleMaxRunes])
	}
	return firstLine
}

func sortSessionsByLastActiveDesc(sessions []*Session) {
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[j].LastActive.Before(sessions[i].LastActive)
	})
}
