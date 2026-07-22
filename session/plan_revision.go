package session

import "time"

type PlanRevision struct {
	Content      string    `json:"content,omitempty"` // full content, initial version only
	Diff         string    `json:"diff,omitempty"`    // unified diff vs previous, revisions only
	Index        int       `json:"index"`
	IsAlteration bool      `json:"is_alteration,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}
