package codex

import (
	"encoding/json"
	"errors"
	"time"
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
