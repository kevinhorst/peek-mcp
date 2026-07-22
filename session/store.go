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

	"github.com/kevinhorst/peek-mcp/state"
	"github.com/pmezard/go-difflib/difflib"
)

const (
	derivedTitleMaxRunes = 80
	maxTitleCandidates   = 5
	maxPlanRevisions     = 50
	maxRevisionDiffBytes = 64 * 1024
)

type Store struct {
	mu sync.RWMutex

	StateDir       *state.Dir
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
	if turn.IsSubagentSignal() {
		s.addSubagentEvents(id, turn)
		return
	}

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

	if turn.FilePath != "" && session.FilePath == "" {
		session.FilePath = turn.FilePath
	}

	for _, event := range turn.Events {
		s.appendEvent(session, event)
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

	if turn.IsUsageSignal() {
		session.TotalUsage = *turn.Usage
		return
	}

	if turn.Role == "" {
		return
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

func (s *Store) addSubagentEvents(id Id, turn *Turn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		slog.Debug("Store.addSubagentEvents: Unknown parent session, dropping events", "session", id)
		return
	}

	for _, event := range turn.Events {
		s.appendEvent(session, event)
	}
}

func (s *Store) appendEvent(session *Session, event *Event) {
	resolveSubagentActor(event, session)
	session.AddEvent(event)
}

func (s *Store) setPlanContent(content string, session *Session, timestamp time.Time) {
	if content == "" || content == session.PlanContent {
		return
	}

	previous := session.PlanContent
	session.PlanContent = content
	s.recordPlanRevision(content, previous, session, timestamp)
}

func (s *Store) recordPlanRevision(current, previous string, session *Session, timestamp time.Time) {
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	if previous == "" {
		initial := &PlanRevision{Content: current, Timestamp: timestamp}
		session.PlanRevisions = append(session.PlanRevisions, initial)
		s.persistPlanVersion(current, initial, session)
		return
	}

	revision := &PlanRevision{
		Diff:         unifiedDiff(current, previous),
		Index:        len(session.PlanRevisions),
		IsAlteration: session.isAlterationPhase(),
		Timestamp:    timestamp,
	}
	if revision.IsAlteration {
		session.Counters.PlanAlterations++
	}

	if len(session.PlanRevisions) < maxPlanRevisions {
		session.PlanRevisions = append(session.PlanRevisions, revision)
		s.persistPlanVersion(current, revision, session)
	}

	planPayload := &PlanPayload{Revision: revision.Index}
	event := &Event{Kind: EventKindPlanRevised, Plan: planPayload, Timestamp: revision.Timestamp}
	s.appendEvent(session, event)
}

func (s *Store) persistPlanVersion(latest string, revision *PlanRevision, session *Session) {
	if s.StateDir == nil || session.Agent != AgentClaude {
		return
	}

	record := &state.PlanVersion{
		Content:      revision.Content,
		Index:        revision.Index,
		IsAlteration: revision.IsAlteration,
	}
	if revision.Index > 0 {
		record.Content = revision.Diff
	}

	agent := string(session.Agent)
	id := string(session.Meta.SessionId)
	if err := s.StateDir.WritePlanVersion(agent, id, record); err != nil {
		slog.Warn("Store.persistPlanVersion: Failed to write plan version", "session", id, "err", err)
	}
	if err := s.StateDir.WritePlanLatest(agent, latest, id); err != nil {
		slog.Warn("Store.persistPlanVersion: Failed to write plan latest", "session", id, "err", err)
	}
}

func (s *Store) UpdateDiff(id Id, target, output string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slog.Debug("Updating diff", "session", id)

	if session, ok := s.sessions[id]; ok {
		session.DiffOutput = output
		session.DiffTarget = target
		session.DiffSource = DiffSourceLive
		session.DiffCapturedAt = time.Now()
	}
}

func (s *Store) MarkDiffSnapshot(id Id) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return
	}
	if session.DiffOutput == "" {
		return
	}

	session.DiffSource = DiffSourceSnapshot
}

func (s *Store) PinDiffBase(id Id, sha, target string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[id]; ok {
		session.DiffBase = sha
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
			s.setPlanContent(content, session, time.Now())
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
		Agent:         agent,
		Events:        NewEventBuffer(EventBufferCapacity),
		Meta:          Meta{SessionId: id},
		TurnsFinished: NewTurnBuffer(s.depth),
	}
	s.hydrateFromState(session)
	s.sessions[id] = session
	return session
}

func (s *Store) hydrateFromState(session *Session) {
	if s.StateDir == nil {
		return
	}

	agent := string(session.Agent)
	id := string(session.Meta.SessionId)

	if base, ok := s.StateDir.ReadDiffBase(agent, id); ok {
		session.DiffBase = base.Sha
		session.DiffTarget = base.Target
	}

	if snapshot, capturedAt, ok := s.StateDir.ReadDiffSnapshot(agent, id); ok {
		session.DiffOutput = snapshot
		session.DiffSource = DiffSourceSnapshot
		session.DiffCapturedAt = capturedAt
	}

	s.hydratePlanState(agent, id, session)
}

func (s *Store) hydratePlanState(agent, id string, session *Session) {
	versions := s.StateDir.ReadPlanVersions(agent, id)
	if len(versions) == 0 {
		return
	}

	for _, version := range versions {
		revision := &PlanRevision{
			Index:        version.Index,
			IsAlteration: version.IsAlteration,
			Timestamp:    version.ModTime,
		}
		if version.Index == 0 {
			revision.Content = version.Content
		} else {
			revision.Diff = version.Content
		}

		session.PlanRevisions = append(session.PlanRevisions, revision)
		if revision.IsAlteration {
			session.Counters.PlanAlterations++
		}
	}

	if latest, ok := s.StateDir.ReadPlanLatest(agent, id); ok {
		session.PlanContent = latest
	}
}

func (s *Store) setTitle(session *Session, title string, source TitleSource) {
	session.Title = title
	session.TitleSource = source
	s.plainTitleById[session.Meta.SessionId] = strings.ToLower(strings.TrimSpace(title))
}

func (s *Store) updatePlanContent(session *Session, turn *Turn) {
	if turn.PlanContent != "" {
		s.setPlanContent(turn.PlanContent, session, turn.Timestamp)
		return
	}

	if content, err := os.ReadFile(turn.PlanFilePath); err == nil {
		s.setPlanContent(string(content), session, turn.Timestamp)
		return
	}

	// Worktree fallback: Claude Code reports plan path as ~/.claude/plans/<name>
	// but worktree sessions write to <cwd>/.claude/plans/<name>.
	if cwd := turn.Meta.CWD; cwd != "" {
		alt := filepath.Join(cwd, ".claude", "plans", filepath.Base(turn.PlanFilePath))
		if content, err := os.ReadFile(alt); err == nil {
			session.PlanFilePath = alt
			s.setPlanContent(string(content), session, turn.Timestamp)
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

func resolveSubagentActor(event *Event, session *Session) {
	if event.Kind != EventKindSubagentResult {
		return
	}
	if event.Subagent == nil || event.Subagent.AgentId != "" {
		return
	}

	for _, seen := range session.Events.All() {
		if seen.Kind != EventKindSubagentSpawned {
			continue
		}
		if seen.Subagent == nil {
			continue
		}
		if seen.Subagent.ToolUseId != event.Subagent.ToolUseId {
			continue
		}

		event.Subagent.AgentId = seen.Subagent.AgentId
		return
	}
}

func unifiedDiff(current, previous string) string {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(previous),
		B:        difflib.SplitLines(current),
		Context:  3,
		FromFile: "previous",
		ToFile:   "current",
	}

	text, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		slog.Warn("unifiedDiff: Failed to compute diff", "err", err)
		return ""
	}

	if len(text) > maxRevisionDiffBytes {
		text = text[:maxRevisionDiffBytes] + "\n[peek: revision diff truncated at 64 KB]\n"
	}
	return text
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
