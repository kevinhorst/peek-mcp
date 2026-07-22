package session

import (
	"time"

	"github.com/pkg/errors"
)

type Turn struct {
	Role            Role        `json:"role"`
	Text            string      `json:"text"` // may be empty
	Timestamp       time.Time   `json:"timestamp"`
	Meta            *Meta       `json:"meta"`
	RequestId       string      `json:"request_id,omitempty"` // optional
	Usage           *Usage      `json:"usage,omitempty"`      // optional
	UsageCumulative bool        `json:"-"`
	PlanFilePath    string      `json:"-"` // plan signal only, not serialized
	PlanContent     string      `json:"-"` // inline plan content from attachment
	CustomTitle     string      `json:"-"` // title signal only, not serialized
	TitleSource     TitleSource `json:"-"`
}

func (t *Turn) Validate() error {
	if t == nil {
		return errors.New("Turn.Validate: called on nil")
	}

	if t.Meta == nil {
		return errors.New("Turn.Validate: meta must not be nil")
	}

	// plan-signal turns only carry a session ID and plan file path
	if t.PlanFilePath != "" {
		if t.Meta.SessionId == "" {
			return errors.New("Turn.Validate: plan signal turn requires session ID")
		}
		return nil
	}

	// title-signal turns only carry a session ID and title
	if t.CustomTitle != "" {
		if t.Meta.SessionId == "" {
			return errors.New("Turn.Validate: title signal turn requires session ID")
		}
		if t.TitleSource == "" {
			return errors.New("Turn.Validate: title signal turn requires title source")
		}
		return nil
	}

	if t.Role != RoleUser && t.Role != RoleAssistant {
		return errors.Errorf("Turn.Validate: role must be \"user\" or \"assistant\": %s", t.Role)
	}

	if t.Timestamp.IsZero() {
		return errors.New("Turn.Validate: timestamp must not be zero")
	}

	if t.Usage != nil {
		if err := t.Usage.Validate(); err != nil {
			return errors.Wrap(err, "Turn.Validate")
		}
	}

	return nil
}
