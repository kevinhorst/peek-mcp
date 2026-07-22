package tools

import (
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

type sessionFullResult struct {
	Diff       string `json:"diff,omitempty"`
	DiffTarget string `json:"diff_target,omitempty"`
	Events     string `json:"events,omitempty"`
	Memory     string `json:"memory,omitempty"`
	Plan       string `json:"plan,omitempty"`
	Turns      string `json:"turns,omitempty"`
}

type sessionFullResultPage struct {
	*sessionFullResult
	RequestId string `json:"request_id,omitempty"`
	HasMore   bool   `json:"has_more"`
}

func newSessionFullResultPage(result *sessionFullResult) *sessionFullResultPage {
	return &sessionFullResultPage{
		sessionFullResult: result,
	}
}

func (p *sessionFullResultPage) WithRequestId(id string) {
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
