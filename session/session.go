package session

import (
	"errors"
	"time"
)

type (
	Id    string
	Agent string
)

const (
	AgentClaude Agent = "claude"
	AgentCodex  Agent = "codex"
)

type Session struct {
	Meta            Meta      `json:"meta"`
	Agent           Agent     `json:"agent"`
	Title           string    `json:"title,omitempty"`
	LastActive      time.Time `json:"last_active"`
	TotalUsage      Usage     `json:"total_usage"`
	FilePath        string    `json:"-"`
	PlanFilePath    string    `json:"-"`
	PlanContent     string    `json:"-"`
	DiffOutput      string    `json:"-"`
	DiffTarget      string    `json:"-"`
	UncommittedDiff string    `json:"-"` // git diff HEAD, refreshed by the poller
	TurnActive      *Turn     `json:"-"`
	TurnsFinished   *TurnBuffer
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
