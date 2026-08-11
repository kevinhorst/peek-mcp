package session

import (
	"errors"
	"time"
)

type (
	Id          string
	Agent       string
	TitleSource string
)

const (
	AgentClaude Agent = "claude"
	AgentCodex  Agent = "codex"
)

const (
	TitleSourceCustom  TitleSource = "custom"
	TitleSourceDerived TitleSource = "derived"
	TitleSourceIndex   TitleSource = "index"
)

const EventBufferCapacity = 500

const idleThreshold = 5 * time.Minute

const maxTouchedFiles = 500

const maxSubagentStats = 200

const maxSkillStats = 100

type DiffSource string

const (
	DiffSourceLive     DiffSource = "live"
	DiffSourceSnapshot DiffSource = "snapshot"
)

type Session struct {
	activeSkill     *SkillStat
	currentPromptId string
	planExitSeen    bool

	Agent           Agent                       `json:"agent"`
	Counters        Counters                    `json:"-"`
	DiffBase        string                      `json:"-"`
	DiffCapturedAt  time.Time                   `json:"-"`
	DiffOutput      string                      `json:"-"`
	DiffSource      DiffSource                  `json:"-"`
	DiffTarget      string                      `json:"diff_target,omitempty"`
	Events          *EventBuffer                `json:"-"`
	FilePath        string                      `json:"-"`
	Idle            time.Duration               `json:"-"`
	LastActive      time.Time                   `json:"last_active"`
	Meta            Meta                        `json:"meta"`
	PlanContent     string                      `json:"-"`
	PlanFilePath    string                      `json:"-"`
	PlanRevisions   []*PlanRevision             `json:"-"`
	Skills          []*SkillStat                `json:"-"`
	StartedAt       time.Time                   `json:"-"`
	Subagents       map[string]*SubagentStat    `json:"-"`
	Title           string                      `json:"title,omitempty"`
	TitleSource     TitleSource                 `json:"title_source,omitempty"`
	TotalUsage      Usage                       `json:"total_usage"`
	TouchedFiles    map[string]*FileTouchCounts `json:"-"`
	TurnActive      *Turn                       `json:"-"`
	TurnsFinished   *TurnBuffer
	UncommittedDiff string `json:"-"`
	usageRequestIds map[string]struct{}
}

func (s *Session) isAlterationPhase() bool {
	if s.Agent == AgentCodex {
		return len(s.PlanRevisions) >= 1
	}
	return s.planExitSeen
}

func (s *Session) AddEvent(event *Event) {
	s.Events.Push(event)

	switch event.Kind {
	case EventKindPermissionDenied:
		s.Counters.PermissionDenials++
	case EventKindPlanModeExit:
		s.planExitSeen = true
	case EventKindPlanRejected:
		s.Counters.PlanRejections++
	case EventKindSkillInvoked:
		s.Counters.SkillsInvoked++
		if event.Skill != nil {
			s.openSkillWindow(event)
		}
	case EventKindSubagentSpawned:
		s.Counters.SubagentsSpawned++
	case EventKindPlanApproved, EventKindPlanModeEnter, EventKindPlanModeReenter,
		EventKindPlanRevised, EventKindSubagentResult, EventKindUserAnswer:
	}
}

type FileTouchCounts struct {
	Reads  int `json:"reads"`
	Writes int `json:"writes"`
}

func (s *Session) AddFileTouch(touch *FileTouch) {
	if s.TouchedFiles == nil {
		s.TouchedFiles = make(map[string]*FileTouchCounts)
	}

	counts, ok := s.TouchedFiles[touch.Path]
	if !ok {
		if len(s.TouchedFiles) >= maxTouchedFiles {
			return
		}
		counts = &FileTouchCounts{}
		s.TouchedFiles[touch.Path] = counts
	}

	if touch.Write {
		counts.Writes++
		return
	}
	counts.Reads++
}

type SkillStat struct {
	Skill     string    `json:"skill"`
	Args      string    `json:"args,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Usage     Usage     `json:"usage"`
}

func (s *Session) HandlePromptBoundary(promptId string, timestamp time.Time) {
	isSameSubmission := promptId != "" && promptId == s.currentPromptId
	s.currentPromptId = promptId
	if isSameSubmission {
		return
	}
	s.CloseSkillWindow(timestamp)
}

func (s *Session) CloseSkillWindow(timestamp time.Time) {
	if s.activeSkill == nil {
		return
	}
	if !timestamp.IsZero() {
		s.activeSkill.EndedAt = timestamp
	}
	s.activeSkill = nil
}

func (s *Session) openSkillWindow(event *Event) {
	s.CloseSkillWindow(event.Timestamp)
	if len(s.Skills) >= maxSkillStats {
		return
	}

	stat := &SkillStat{
		Skill:     event.Skill.Skill,
		Args:      event.Skill.Args,
		StartedAt: event.Timestamp,
	}
	s.Skills = append(s.Skills, stat)
	s.activeSkill = stat
}

type SubagentStat struct {
	AgentType   string    `json:"agent_type,omitempty"`
	Description string    `json:"description,omitempty"`
	FirstActive time.Time `json:"first_active"`
	LastActive  time.Time `json:"last_active"`
	Usage       Usage     `json:"usage"`
}

func (s *Session) AddSubagentTurn(turn *Turn) {
	if s.Subagents == nil {
		s.Subagents = make(map[string]*SubagentStat)
	}

	stat, ok := s.Subagents[turn.SubagentId]
	if !ok {
		if len(s.Subagents) >= maxSubagentStats {
			return
		}
		stat = &SubagentStat{}
		s.Subagents[turn.SubagentId] = stat
	}

	if !turn.Timestamp.IsZero() {
		if stat.FirstActive.IsZero() {
			stat.FirstActive = turn.Timestamp
		}
		stat.LastActive = turn.Timestamp
	}

	for _, event := range turn.Events {
		if event.Kind == EventKindSubagentSpawned && event.Subagent != nil {
			stat.AgentType = event.Subagent.AgentType
			stat.Description = event.Subagent.Description
		}
	}

	if turn.Usage == nil || turn.RequestId == "" {
		return
	}
	if s.usageRequestIds == nil {
		s.usageRequestIds = make(map[string]struct{})
	}
	if _, counted := s.usageRequestIds[turn.RequestId]; counted {
		return
	}
	s.usageRequestIds[turn.RequestId] = struct{}{}
	stat.Usage.Add(turn.Usage)
}

func (s *Session) AddTurn(nextTurn *Turn) {
	// always update meta info
	s.Meta.Update(nextTurn.Meta)

	if !nextTurn.Timestamp.IsZero() {
		if s.StartedAt.IsZero() {
			s.StartedAt = nextTurn.Timestamp
		}
		if gap := nextTurn.Timestamp.Sub(s.LastActive); !s.LastActive.IsZero() && gap >= idleThreshold {
			s.Idle += gap
		}
		s.LastActive = nextTurn.Timestamp
	}

	if nextTurn.Usage != nil && nextTurn.RequestId != "" {
		if s.usageRequestIds == nil {
			s.usageRequestIds = make(map[string]struct{})
		}
		if _, counted := s.usageRequestIds[nextTurn.RequestId]; !counted {
			s.usageRequestIds[nextTurn.RequestId] = struct{}{}
			s.TotalUsage.Add(nextTurn.Usage)
			if s.activeSkill != nil {
				s.activeSkill.Usage.Add(nextTurn.Usage)
				s.activeSkill.EndedAt = nextTurn.Timestamp
			}
		}
	}

	// first turn
	if s.TurnActive == nil {
		s.TurnActive = nextTurn
		return
	}

	// same turn, append text, no-op for empty text
	if nextTurn.RequestId != "" && s.TurnActive.RequestId == nextTurn.RequestId {
		merged := *nextTurn
		merged.Text = s.TurnActive.Text + nextTurn.Text
		s.TurnActive = &merged
		return
	}

	if s.TurnActive.Text != "" {
		s.TurnsFinished.Push(s.TurnActive)
	}

	s.TurnActive = nextTurn
}

func (s *Session) CurrentUsage() *Usage {
	total := s.TotalUsage
	if s.TurnActive != nil {
		total.Add(s.TurnActive.Usage)
	}
	return &total
}

func (s *Session) HasNewTitle(title string, source TitleSource) bool {
	if title == "" {
		return false
	}

	if titleSourceRank(source) < titleSourceRank(s.TitleSource) {
		return false
	}

	return s.Title != title
}

func (s *Session) Turns(number int) []*Turn {
	if s.TurnActive == nil {
		return s.TurnsFinished.Last(number)
	}

	buffer := &TurnBuffer{
		capacity: s.TurnsFinished.capacity,
		items:    append([]*Turn{s.TurnActive}, s.TurnsFinished.items...),
	}

	return buffer.Last(number)
}

func (s *Session) Validate() error {
	if s == nil {
		return errors.New("session is nil")
	}

	if s.Meta.SessionId == "" {
		return errors.New("id must not be empty")
	}

	if s.Agent != AgentClaude && s.Agent != AgentCodex {
		return errors.New("agent must be \"claude\" or \"codex\"")
	}

	if s.LastActive.IsZero() {
		return errors.New("last_active must not be zero")
	}

	if s.TurnsFinished == nil {
		return errors.New("turns must not be nil")
	}

	if s.Events == nil {
		return errors.New("events must not be nil")
	}

	return nil
}

func titleSourceRank(source TitleSource) int {
	switch source {
	case TitleSourceCustom:
		return 2
	case TitleSourceIndex:
		return 1
	case TitleSourceDerived:
		return 0
	}
	return 0
}
