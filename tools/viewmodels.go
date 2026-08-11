package tools

import (
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

type sessionGetResult struct {
	Diff            string         `json:"diff,omitempty"`
	DiffTarget      string         `json:"diff_target,omitempty"`
	Events          any            `json:"events,omitempty"`
	Memory          any            `json:"memory,omitempty"`
	Plan            string         `json:"plan,omitempty"`
	TotalUsage      *session.Usage `json:"total_usage,omitempty"`
	Turns           any            `json:"turns,omitempty"`
	UncommittedDiff string         `json:"uncommitted_diff,omitempty"`
}

type sessionGetResultPage struct {
	*sessionGetResult
	HasMore   bool   `json:"has_more"`
	RequestId string `json:"request_id,omitempty"`
}

func newSessionGetResultPage(result *sessionGetResult) *sessionGetResultPage {
	return &sessionGetResultPage{
		sessionGetResult: result,
	}
}

func (p *sessionGetResultPage) WithRequestId(id string) {
	p.HasMore = true
	p.RequestId = id
}

type sessionListItem struct {
	Id          session.Id          `json:"id"`
	Agent       session.Agent       `json:"agent"`
	Title       string              `json:"title,omitempty"`
	TitleSource session.TitleSource `json:"title_source,omitempty"`
	LastActive  time.Time           `json:"last_active"`
	HasPlan     bool                `json:"has_plan"`
	HasDiff     bool                `json:"has_diff"`
	DiffTarget  string              `json:"diff_target,omitempty"`
	Meta        session.Meta        `json:"meta"`
}
