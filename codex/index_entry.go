package codex

import (
	"errors"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

type IndexEntry struct {
	Id         session.Id `json:"id"`
	ThreadName string     `json:"thread_name"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (e *IndexEntry) Validate() error {
	if e == nil {
		return errors.New("codex index entry is nil")
	}

	// Id
	if e.Id == "" {
		return errors.New("id must not be empty")
	}

	// ThreadName
	if e.ThreadName == "" {
		return errors.New("thread_name must not be empty")
	}

	return nil
}
