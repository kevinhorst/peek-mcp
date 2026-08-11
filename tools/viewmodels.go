package tools

import (
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

type sessionGetResult struct {
	Turns           string `json:"turns,omitempty"`
	Plan            string `json:"plan,omitempty"`
	Diff            string `json:"diff,omitempty"`
	DiffTarget      string `json:"diff_target,omitempty"`
	UncommittedDiff string `json:"uncommitted_diff,omitempty"`
}

type sessionGetResultPage struct {
	*sessionGetResult
	RequestId string `json:"request_id,omitempty"`
	HasMore   bool   `json:"has_more"`
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
