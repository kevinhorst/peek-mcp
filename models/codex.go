package models

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	CodexEntryTypeSessionMeta  = "session_meta"
	CodexEntryTypeTurnContext  = "turn_context"
	CodexEntryTypeResponseItem = "response_item"
)

type CodexEntry struct {
	Timestamp time.Time       `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

func (e *CodexEntry) Validate() error {
	if e == nil {
		return errors.New("codex entry is nil")
	}
	if e.Type == "" {
		return errors.New("type must not be empty")
	}
	if len(e.Payload) == 0 {
		return errors.New("payload must not be empty")
	}
	return nil
}

type CodexSessionMeta struct {
	ID         string        `json:"id"`
	CWD        string        `json:"cwd"`
	CLIVersion string        `json:"cli_version"`
	Git        *CodexGitInfo `json:"git"`
}

func (m *CodexSessionMeta) Validate() error {
	if m == nil {
		return errors.New("codex session meta is nil")
	}
	if m.ID == "" {
		return errors.New("id must not be empty")
	}
	return nil
}

type CodexGitInfo struct {
	CommitHash    string `json:"commit_hash"`
	RepositoryURL string `json:"repository_url"`
}

type CodexTurnContext struct {
	TurnID string `json:"turn_id"`
	Model  string `json:"model"`
	CWD    string `json:"cwd"`
}

func (c *CodexTurnContext) Validate() error {
	if c == nil {
		return errors.New("codex turn context is nil")
	}
	if c.TurnID == "" && c.Model == "" && c.CWD == "" {
		return errors.New("turn context must include turn_id, model, or cwd")
	}
	return nil
}

type CodexResponseItem struct {
	Type    string              `json:"type"`
	Role    string              `json:"role"`
	Content []CodexContentBlock `json:"content"`
}

func (i *CodexResponseItem) Validate() error {
	if i == nil {
		return errors.New("codex response item is nil")
	}
	if i.Type == "" {
		return errors.New("type must not be empty")
	}
	if i.Role != "" && i.Role != "user" && i.Role != "assistant" {
		return errors.New("role must be empty, \"user\", or \"assistant\"")
	}
	return nil
}

type CodexContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (b *CodexContentBlock) Validate() error {
	if b == nil {
		return errors.New("codex content block is nil")
	}
	if b.Type == "" {
		return errors.New("type must not be empty")
	}
	return nil
}
