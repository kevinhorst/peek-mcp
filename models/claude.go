package models

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	ClaudeEntryTypeUser      = "user"
	ClaudeEntryTypeAssistant = "assistant"
)

type ClaudeEntry struct {
	Type        string          `json:"type"`
	SessionID   string          `json:"sessionId"`
	Timestamp   time.Time       `json:"timestamp"`
	CWD         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	IsSidechain bool            `json:"isSidechain"`
	PromptID    string          `json:"promptId"`
	RequestID   string          `json:"requestId"`
	Message     json.RawMessage `json:"message"`
}

func (e *ClaudeEntry) Validate() error {
	if e == nil {
		return errors.New("claude entry is nil")
	}
	if e.Type == "" {
		return errors.New("type must not be empty")
	}
	if (e.Type == ClaudeEntryTypeUser || e.Type == ClaudeEntryTypeAssistant) && e.SessionID == "" {
		return errors.New("session_id must not be empty")
	}
	return nil
}

type ClaudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Model   string          `json:"model"`
	Usage   *ClaudeUsage    `json:"usage"`
}

func (m *ClaudeMessage) Validate() error {
	if m == nil {
		return errors.New("claude message is nil")
	}
	if m.Role != "" && m.Role != "user" && m.Role != "assistant" {
		return errors.New("role must be empty, \"user\", or \"assistant\"")
	}
	if m.Usage != nil {
		if err := m.Usage.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (u *ClaudeUsage) Validate() error {
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

type ClaudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (b *ClaudeContentBlock) Validate() error {
	if b == nil {
		return errors.New("claude content block is nil")
	}
	if b.Type == "" {
		return errors.New("type must not be empty")
	}
	return nil
}
