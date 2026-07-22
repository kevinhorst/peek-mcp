package claude

import (
	"encoding/json"
	"errors"
)

type ContentBlock struct {
	Id        string          `json:"id"`
	Content   json.RawMessage `json:"content"`
	Input     json.RawMessage `json:"input"`
	IsError   bool            `json:"is_error"`
	Name      string          `json:"name"`
	Text      string          `json:"text"`
	ToolUseId string          `json:"tool_use_id"`
	Type      string          `json:"type"`
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
