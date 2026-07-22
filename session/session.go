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

type DiffSource string

const (
	DiffSourceLive     DiffSource = "live"
	DiffSourceSnapshot DiffSource = "snapshot"
)

type Session struct {
	planExitSeen bool

	Agent           Agent           `json:"agent"`
	Counters        Counters        `json:"-"`
	DiffBase        string          `json:"-"`
	DiffCapturedAt  time.Time       `json:"-"`
	DiffOutput      string          `json:"-"`
	DiffSource      DiffSource      `json:"-"`
	DiffTarget      string          `json:"diff_target,omitempty"`
	Events          *EventBuffer    `json:"-"`
	FilePath        string          `json:"-"`
	LastActive      time.Time       `json:"last_active"`
	Meta            Meta            `json:"meta"`
	PlanContent     string          `json:"-"`
	PlanFilePath    string          `json:"-"`
	PlanRevisions   []*PlanRevision `json:"-"`
	Title           string          `json:"title,omitempty"`
	TitleSource     TitleSource     `json:"title_source,omitempty"`
	TotalUsage      Usage           `json:"total_usage"`
	TurnActive      *Turn           `json:"-"`
	TurnsFinished   *TurnBuffer
	UncommittedDiff string `json:"-"`
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
	case EventKindSubagentSpawned:
		s.Counters.SubagentsSpawned++
	case EventKindPlanApproved, EventKindPlanModeEnter, EventKindPlanModeReenter,
		EventKindPlanRevised, EventKindSubagentResult, EventKindUserAnswer:
	}
}

func (s *Session) AddTurn(nextTurn *Turn) {
	// always update meta info
	s.Meta.Update(nextTurn.Meta)

	if !nextTurn.Timestamp.IsZero() {
		s.LastActive = nextTurn.Timestamp
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

	// new turn, update usage and push old turn
	if s.TurnActive.Usage != nil {
		s.TotalUsage.Add(s.TurnActive.Usage)
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
