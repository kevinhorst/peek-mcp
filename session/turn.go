package session

import (
	"time"

	"github.com/pkg/errors"
)

type Turn struct {
	Role      Role      `json:"role"`
	Text      string    `json:"text"` // optional
	Timestamp time.Time `json:"timestamp"`
	Meta      *Meta     `json:"meta"`
	RequestId string    `json:"request_id,omitempty"`
	Usage     *Usage    `json:"usage,omitempty"`
}

func (t *Turn) Validate() error {
	if t == nil {
		return errors.New("Turn.Validate: called on nil")
	}

	if t.Role != RoleUser && t.Role != RoleAssistant {
		return errors.New("Turn.Validate: role must be \"user\" or \"assistant\"")
	}

	if t.Timestamp.IsZero() {
		return errors.New("Turn.Validate: timestamp must not be zero")
	}

	if t.Meta == nil {
		return errors.New("Turn.Validate: meta must not be nil")
	}

	if t.Usage != nil {
		if err := t.Usage.Validate(); err != nil {
			return errors.Wrap(err, "Turn.Validate")
		}
	}

	return nil
}
