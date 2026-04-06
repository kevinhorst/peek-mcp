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
	Id                Id        `json:"id"`
	Source            Source    `json:"source"`
	CurrentWorkingDir string    `json:"cwd,omitempty"`
	GitBranch         string    `json:"git_branch,omitempty"`
	Model             string    `json:"model,omitempty"`
	LastActive        time.Time `json:"last_active"`
	TotalUsage        Usage     `json:"total_usage"`
	FilePath          string    `json:"-"`
	Turns             *TurnBuffer
}

func (s *Session) Validate() error {
	if s == nil {
		return errors.New("session is nil")
	}

	if s.Id == "" {
		return errors.New("id must not be empty")
	}

	if s.Source != SourceClaude && s.Source != SourceCodex {
		return errors.New("source must be \"claude\" or \"codex\"")
	}

	if s.LastActive.IsZero() {
		return errors.New("last_active must not be zero")
	}

	if s.Turns == nil {
		return errors.New("turns must not be nil")
	}

	return nil
}
