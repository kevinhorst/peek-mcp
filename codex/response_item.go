package codex

import (
	"encoding/json"
	"errors"

	"github.com/kevinhorst/peek-mcp/session"
)

type ResponseItem struct {
	Arguments json.RawMessage `json:"arguments"`
	CallId    string          `json:"call_id"`
	Content   []ContentBlock  `json:"content"`
	Name      string          `json:"name"`
	Output    json.RawMessage `json:"output"`
	Role      session.Role    `json:"role"`
	Type      string          `json:"type"`
}

func (i *ResponseItem) Validate() error {
	if i == nil {
		return errors.New("codex response item is nil")
	}

	if i.Type == "" {
		return errors.New("type must not be empty")
	}

	if i.Role != "" && i.Role != session.RoleUser && i.Role != session.RoleAssistant && i.Role != session.RoleDeveloper {
		return errors.New("role must be empty, \"user\", \"assistant\", or \"developer\"")
	}

	return nil
}
