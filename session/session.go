package session

import (
	"errors"
	"time"
)

type (
	Id     string
	Source string
)

const (
	SourceClaude Source = "claude"
	SourceCodex  Source = "codex"
)

type Session struct {
	Meta          Meta      `json:"meta"`
	Source        Source    `json:"source"`
	LastActive    time.Time `json:"last_active"`
	TotalUsage    Usage     `json:"total_usage"`
	FilePath      string    `json:"-"`
	TurnActive    *Turn     `json:"-"`
	TurnsFinished *TurnBuffer
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

func (s *Session) Update(nextTurn *Turn) {
	// update meta
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

	if s.Source != SourceClaude && s.Source != SourceCodex {
		return errors.New("source must be \"claude\" or \"codex\"")
	}

	if s.LastActive.IsZero() {
		return errors.New("last_active must not be zero")
	}

	if s.TurnsFinished == nil {
		return errors.New("turns must not be nil")
	}

	return nil
}
