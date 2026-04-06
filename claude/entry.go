package claude

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/kevinhorst/peek-mcp/models"
)

const (
	EntryTypeUser      = "user"
	EntryTypeAssistant = "assistant"
)

type Entry struct {
	CWD         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
	PromptID    string          `json:"promptId"`
	RequestID   string          `json:"requestId"`
	SessionID   string          `json:"sessionId"`
	Timestamp   time.Time       `json:"timestamp"`
	Type        string          `json:"type"`
}

func (e *Entry) Validate() error {
	if e == nil {
		return errors.New("claude entry is nil")
	}
	if e.Type == "" {
		return errors.New("type must not be empty")
	}
	if (e.Type == EntryTypeUser || e.Type == EntryTypeAssistant) && e.SessionID == "" {
		return errors.New("session_id must not be empty")
	}
	return nil
}

type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Model   string          `json:"model"`
	Usage   *Usage          `json:"usage"`
}

func (m *Message) Validate() error {
	if m == nil {
		return errors.New("claude message is nil")
	}
	if m.Role != "" && m.Role != models.RoleUser && m.Role != models.RoleAssistant {
		return errors.New("role must be empty, \"user\", or \"assistant\"")
	}
	if m.Usage != nil {
		if err := m.Usage.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (u *Usage) Validate() error {
	if u == nil {
		return errors.New("claude usage is nil")
	}

	if u.InputTokens < 0 {
		return errors.New("input_tokens must be non-negative")
	}

	if u.OutputTokens < 0 {
		return errors.New("output_tokens must be non-negative")
	}

	if u.CacheCreationInputTokens < 0 {
		return errors.New("cache_creation_input_tokens must be non-negative")
	}

	if u.CacheReadInputTokens < 0 {
		return errors.New("cache_read_input_tokens must be non-negative")
	}

	return nil
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (b *ContentBlock) Validate() error {
	if b == nil {
		return errors.New("claude content block is nil")
	}
	if b.Type == "" {
		return errors.New("type must not be empty")
	}
	return nil
}
