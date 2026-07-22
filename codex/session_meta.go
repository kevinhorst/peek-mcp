package codex

import (
	"encoding/json"
	"errors"

	"github.com/kevinhorst/peek-mcp/session"
)

const (
	SourceKindSubagent = "subagent"
	SourceKindUnknown  = "unknown"
)

type GitInfo struct {
	Branch        string `json:"branch"`
	CommitHash    string `json:"commit_hash"`
	RepositoryURL string `json:"repository_url"`
}

type SessionMeta struct {
	Id           session.Id `json:"id"`
	CLIVersion   string     `json:"cli_version"`
	CWD          string     `json:"cwd"`
	ForkedFromId string     `json:"forked_from_id"`
	Git          *GitInfo   `json:"git"`
	Originator   string     `json:"originator"`
	Source       Source     `json:"source"`
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

// Source is string-or-object in rollouts: "vscode" / "mcp" for normal
// sessions, an object carrying subagent.thread_spawn for sub-agent rollouts.
type Source struct {
	AgentNickname  string
	Kind           string
	ParentThreadId string
}

func (s *Source) IsSubagent() bool {
	return s.Kind == SourceKindSubagent
}

// UnmarshalJSON never returns an error: a malformed source must degrade to
// "unknown", not fail the whole session_meta (a dropped normal session).
func (s *Source) UnmarshalJSON(data []byte) error {
	var kind string
	if err := json.Unmarshal(data, &kind); err == nil {
		s.Kind = kind
		return nil
	}

	var object sourceObject
	err := json.Unmarshal(data, &object)
	if err != nil || object.Subagent == nil || object.Subagent.ThreadSpawn == nil {
		s.Kind = SourceKindUnknown
		return nil
	}

	s.AgentNickname = object.Subagent.ThreadSpawn.AgentNickname
	s.Kind = SourceKindSubagent
	s.ParentThreadId = object.Subagent.ThreadSpawn.ParentThreadId
	return nil
}

type sourceObject struct {
	Subagent *sourceSubagent `json:"subagent"`
}

type sourceSubagent struct {
	ThreadSpawn *sourceThreadSpawn `json:"thread_spawn"`
}

type sourceThreadSpawn struct {
	AgentNickname  string `json:"agent_nickname"`
	ParentThreadId string `json:"parent_thread_id"`
}
