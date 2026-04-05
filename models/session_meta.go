package models

import (
	"errors"
	"time"
)

type SessionMeta struct {
	ID         string    `json:"id"`
	Source     string    `json:"source"`
	CWD        string    `json:"cwd,omitempty"`
	GitBranch  string    `json:"git_branch,omitempty"`
	Model      string    `json:"model,omitempty"`
	LastActive time.Time `json:"last_active"`
	TotalUsage Usage     `json:"total_usage"`
	FilePath   string    `json:"-"`
}

func (m *SessionMeta) Validate() error {
	if m == nil {
		return errors.New("session meta is nil")
	}
	if m.ID == "" {
		return errors.New("id must not be empty")
	}
	if m.Source != "claude" && m.Source != "codex" {
		return errors.New("source must be \"claude\" or \"codex\"")
	}
	if m.LastActive.IsZero() {
		return errors.New("last_active must not be zero")
	}
	return nil
}
