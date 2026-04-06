package codex

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/kevinhorst/peek-mcp/models"
)

const (
	EntryTypeSessionMeta  = "session_meta"
	EntryTypeTurnContext  = "turn_context"
	EntryTypeResponseItem = "response_item"
	EntryTypeEventMessage = "event_msg"

	ResponseItemTypeMessage = "message"
	EventTypeTokenCount     = "token_count"
)

type Entry struct {
	Timestamp time.Time       `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

func (e *Entry) Validate() error {
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

type SessionMeta struct {
	ID         string   `json:"id"`
	CWD        string   `json:"cwd"`
	CLIVersion string   `json:"cli_version"`
	Git        *GitInfo `json:"git"`
}

func (m *SessionMeta) Validate() error {
	if m == nil {
		return errors.New("codex session meta is nil")
	}
	if m.ID == "" {
		return errors.New("id must not be empty")
	}
	return nil
}

type GitInfo struct {
	CommitHash    string `json:"commit_hash"`
	RepositoryURL string `json:"repository_url"`
}

type TurnContext struct {
	TurnID string `json:"turn_id"`
	Model  string `json:"model"`
	CWD    string `json:"cwd"`
}

func (c *TurnContext) Validate() error {
	if c == nil {
		return errors.New("codex turn context is nil")
	}
	if c.TurnID == "" && c.Model == "" && c.CWD == "" {
		return errors.New("turn context must include turn_id, model, or cwd")
	}
	return nil
}

type ResponseItem struct {
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

func (i *ResponseItem) Validate() error {
	if i == nil {
		return errors.New("codex response item is nil")
	}
	if i.Type == "" {
		return errors.New("type must not be empty")
	}
	if i.Role != "" && i.Role != models.RoleUser && i.Role != models.RoleAssistant && i.Role != models.RoleDeveloper {
		return errors.New("role must be empty, \"user\", \"assistant\", or \"developer\"")
	}
	return nil
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (b *ContentBlock) Validate() error {
	if b == nil {
		return errors.New("codex content block is nil")
	}
	if b.Type == "" {
		return errors.New("type must not be empty")
	}
	return nil
}

type EventMessage struct {
	Type string     `json:"type"`
	Info *EventInfo `json:"info"`
}

func (m *EventMessage) Validate() error {
	if m == nil {
		return errors.New("codex event message is nil")
	}
	if m.Type == "" {
		return errors.New("type must not be empty")
	}
	if m.Type == EventTypeTokenCount && m.Info != nil && m.Info.TotalTokenUsage != nil {
		if err := m.Info.TotalTokenUsage.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type EventInfo struct {
	TotalTokenUsage *TokenUsage `json:"total_token_usage"`
}

type TokenUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
	TotalTokens           int `json:"total_tokens"`
}

func (u *TokenUsage) Validate() error {
	if u == nil {
		return errors.New("codex token usage is nil")
	}
	if u.InputTokens < 0 {
		return errors.New("input_tokens must be non-negative")
	}
	if u.CachedInputTokens < 0 {
		return errors.New("cached_input_tokens must be non-negative")
	}
	if u.OutputTokens < 0 {
		return errors.New("output_tokens must be non-negative")
	}
	if u.ReasoningOutputTokens < 0 {
		return errors.New("reasoning_output_tokens must be non-negative")
	}
	if u.TotalTokens < 0 {
		return errors.New("total_tokens must be non-negative")
	}
	return nil
}
