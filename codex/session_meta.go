package codex

import (
	"errors"

	"github.com/kevinhorst/peek-mcp/session"
)

type SessionMeta struct {
	Id         session.Id `json:"id"`
	CWD        string     `json:"cwd"`
	CLIVersion string     `json:"cli_version"`
	Git        *GitInfo   `json:"git"`
}

func (m *SessionMeta) Validate() error {
	if m == nil {
		return errors.New("codex session meta is nil")
	}

	if m.Id == "" {
		return errors.New("id must not be empty")
	}

	return nil
}

type GitInfo struct {
	CommitHash    string `json:"commit_hash"`
	RepositoryURL string `json:"repository_url"`
}
