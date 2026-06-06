package tools

import (
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

type sessionFullResult struct {
	Turns []*session.Turn `json:"turns,omitempty"`
	Plan  string          `json:"plan,omitempty"`
	Diff  string          `json:"diff,omitempty"`
}

type sessionFullResultPaginated struct {
	Session   *sessionFullResult `json:"session"`
	RequestId string             `json:"request_id,omitempty"`
	HasMore   bool               `json:"has_more,omitempty"`
}

type sessionListItem struct {
	Id         session.Id    `json:"id"`
	Agent      session.Agent `json:"agent"`
	LastActive time.Time     `json:"last_active"`
	HasPlan    bool          `json:"has_plan"`
	HasDiff    bool          `json:"has_diff"`
}
