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
	Info  *Info
	Turns *TurnBuffer
}

func (s *Session) Validate() error {
	if s == nil {
		return errors.New("session is nil")
	}

	if s.Info == nil {
		return errors.New("session meta must not be nil")
	}

	if err := s.Info.Validate(); err != nil {
		return err
	}

	if s.Turns == nil {
		return errors.New("turns must not be nil")
	}

	return nil
}

type Info struct {
	Id         Id        `json:"id"`
	Source     Source    `json:"source"`
	CWD        string    `json:"cwd,omitempty"`
	GitBranch  string    `json:"git_branch,omitempty"`
	Model      string    `json:"model,omitempty"`
	LastActive time.Time `json:"last_active"`
	TotalUsage Usage     `json:"total_usage"`
	FilePath   string    `json:"-"`
}

func (m *Info) Validate() error {
	if m == nil {
		return errors.New("session meta is nil")
	}

	if m.Id == "" {
		return errors.New("id must not be empty")
	}

	if m.Source != SourceClaude && m.Source != SourceCodex {
		return errors.New("source must be \"claude\" or \"codex\"")
	}

	if m.LastActive.IsZero() {
		return errors.New("last_active must not be zero")
	}
	return nil
}
