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
	Subagents       []subagentRef  `json:"subagents,omitempty"`
	TotalUsage      *session.Usage `json:"total_usage,omitempty"`
	Turns           any            `json:"turns,omitempty"`
	TurnsTotal      *int           `json:"turns_total,omitempty"`
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

type subagentRef struct {
	AgentId     string `json:"agent_id"`
	AgentType   string `json:"agent_type,omitempty"`
	Description string `json:"description,omitempty"`
}

func newSubagentRefs(sess *session.Session) []subagentRef {
	refs := make([]subagentRef, 0, len(sess.Subagents))
	for _, id := range sess.SubagentIds() {
		stat := sess.Subagents[id]
		refs = append(refs, subagentRef{AgentId: id, AgentType: stat.AgentType, Description: stat.Description})
	}
	return refs
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

type turnView struct {
	*session.Turn
	ToolCalls []*session.ToolCall `json:"tool_calls,omitzero"`
}

func newTurnViews(turns []*session.Turn, withThinking, withTools bool) []*turnView {
	views := make([]*turnView, len(turns))
	for index, turn := range turns {
		copied := *turn
		copied.ToolCalls = nil
		if !withThinking {
			copied.Thinking = ""
		}
		views[index] = &turnView{ToolCalls: toolCallsForOutput(turn, withTools), Turn: &copied}
	}
	return views
}

func toolCallsForOutput(turn *session.Turn, withTools bool) []*session.ToolCall {
	if !withTools {
		return nil
	}

	calls := make([]*session.ToolCall, len(turn.ToolCalls))
	for index, call := range turn.ToolCalls {
		copied := *call
		calls[index] = &copied
	}
	return calls
}
