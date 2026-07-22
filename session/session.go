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

type Session struct {
	Meta            Meta        `json:"meta"`
	Agent           Agent       `json:"agent"`
	Title           string      `json:"title,omitempty"`
	TitleSource     TitleSource `json:"title_source,omitempty"`
	LastActive      time.Time   `json:"last_active"`
	TotalUsage      Usage       `json:"total_usage"`
	FilePath        string      `json:"-"`
	PlanFilePath    string      `json:"-"`
	PlanContent     string      `json:"-"`
	DiffOutput      string      `json:"-"`
	DiffTarget      string      `json:"diff_target,omitempty"`
	UncommittedDiff string      `json:"-"` // git diff HEAD, refreshed by the poller
	TurnActive      *Turn       `json:"-"`
	TurnsFinished   *TurnBuffer
	usageRequestIds map[string]struct{}
}

func (s *Session) AddTurn(nextTurn *Turn) {
	// always update meta info
	s.Meta.Update(nextTurn.Meta)

	if !nextTurn.Timestamp.IsZero() {
		s.LastActive = nextTurn.Timestamp
	}

	if nextTurn.Usage != nil && nextTurn.UsageCumulative {
		s.TotalUsage = *nextTurn.Usage
		return
	}

	if nextTurn.Usage != nil && nextTurn.RequestId != "" {
		if s.usageRequestIds == nil {
			s.usageRequestIds = make(map[string]struct{})
		}
		if _, counted := s.usageRequestIds[nextTurn.RequestId]; !counted {
			s.usageRequestIds[nextTurn.RequestId] = struct{}{}
			s.TotalUsage.Add(nextTurn.Usage)
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
