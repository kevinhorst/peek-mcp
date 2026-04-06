package session

import (
	"errors"
	"time"
)

type Turn struct {
	Role      Role      `json:"role"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model,omitempty"`
	Usage     *Usage    `json:"usage,omitempty"`
}

func (t *Turn) Validate() error {
	if t == nil {
		return errors.New("turn is nil")
	}

	if t.Role != RoleUser && t.Role != RoleAssistant {
		return errors.New("role must be \"user\" or \"assistant\"")
	}

	if t.Text == "" {
		return errors.New("text must not be empty")
	}

	if t.Timestamp.IsZero() {
		return errors.New("timestamp must not be zero")
	}

	if t.Usage != nil {
		if err := t.Usage.Validate(); err != nil {
			return err
		}
	}

	return nil
}
