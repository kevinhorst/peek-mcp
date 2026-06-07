package tools

import (
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

type sessionFullResult struct {
	Turns string `json:"turns,omitempty"`
	Plan  string `json:"plan,omitempty"`
	Diff  string `json:"diff,omitempty"`
}

type sessionFullResultPage struct {
	*sessionFullResult
	RequestId string `json:"request_id,omitempty"`
	HasMore   bool   `json:"has_more,omitempty"`
}

func newSessionFullResultPage(result *sessionFullResult) *sessionFullResultPage {
	return &sessionFullResultPage{
		sessionFullResult: result,
	}
}

type sessionListItem struct {
	Id         session.Id    `json:"id"`
	Agent      session.Agent `json:"agent"`
	LastActive time.Time     `json:"last_active"`
	HasPlan    bool          `json:"has_plan"`
	HasDiff    bool          `json:"has_diff"`
}
