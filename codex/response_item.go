package codex

import (
	"errors"

	"github.com/kevinhorst/peek-mcp/session"
)

type ResponseItem struct {
	Type    string         `json:"type"`
	Role    session.Role   `json:"role"`
	Content []ContentBlock `json:"content"`
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
