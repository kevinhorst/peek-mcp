package claude

import (
	"encoding/json"
	"errors"
	"time"
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
