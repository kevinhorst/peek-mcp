package claude

import (
	"encoding/json"
	"errors"

	"github.com/kevinhorst/peek-mcp/session"
)

type Message struct {
	Role    session.Role    `json:"role"`
	Content json.RawMessage `json:"content"`
	Model   string          `json:"model"`
	Usage   *Usage          `json:"usage"`
}

func (m *Message) Validate() error {
	if m == nil {
		return errors.New("claude message is nil")
	}

	if m.Role != "" && m.Role != session.RoleUser && m.Role != session.RoleAssistant {
		return errors.New("role must be empty, \"user\", or \"assistant\"")
	}

	if m.Usage != nil {
		if err := m.Usage.Validate(); err != nil {
			return err
		}
	}

	return nil
}
