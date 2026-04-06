package claude

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

const (
	EntryTypeUser      = "user"
	EntryTypeAssistant = "assistant"
)

type Entry struct {
	CurrentWorkingDir string          `json:"cwd"`
	GitBranch         string          `json:"gitBranch"`
	IsSidechain       bool            `json:"isSidechain"`
	Message           json.RawMessage `json:"message"`
	PromptId          string          `json:"promptId"`
	RequestI          string          `json:"requestId"`
	SessionId         session.Id      `json:"sessionId"`
	Timestamp         time.Time       `json:"timestamp"`
	Type              string          `json:"type"`
}

func (e *Entry) Validate() error {
	if e == nil {
		return errors.New("claude entry is nil")
	}
	if e.Type == "" {
		return errors.New("type must not be empty")
	}
	if (e.Type == EntryTypeUser || e.Type == EntryTypeAssistant) && e.SessionId == "" {
		return errors.New("session_id must not be empty")
	}
	return nil
}
