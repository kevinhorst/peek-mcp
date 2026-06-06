package tools

import (
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

type sessionFullResult struct {
	Turns     []*session.Turn `json:"turns"`
	Plan      string          `json:"plan,omitempty"`
	Diff      string          `json:"diff,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
}

type sessionListItem struct {
	Id         session.Id    `json:"id"`
	Agent      session.Agent `json:"agent"`
	Title      string        `json:"title,omitempty"`
	LastActive time.Time     `json:"last_active"`
	HasPlan    bool          `json:"has_plan"`
	HasDiff    bool          `json:"has_diff"`
}
