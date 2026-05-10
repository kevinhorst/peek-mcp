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

func (s *Session) Update(turn *Turn) {
	if s.TurnActive != nil {
		if s.TurnActive.RequestId == turn.RequestId {
			s.TurnActive = turn
			return
		}
		if s.TurnActive.Text != "" {
			s.TurnsFinished.Push(s.TurnActive)
		}
	}
	s.TurnActive = turn

	s.Meta.Update(turn.Meta)

	if !turn.Timestamp.IsZero() {
		s.LastActive = turn.Timestamp
	}

	if turn.Usage != nil {
		s.TotalUsage.Add(turn.Usage)
	}
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
