package control

import (
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

type sessionSummary struct {
	Id                 session.Id    `json:"id"`
	Agent              session.Agent `json:"agent"`
	Title              string        `json:"title,omitempty"`
	LastActive         time.Time     `json:"last_active"`
	CWD                string        `json:"cwd,omitempty"`
	GitBranch          string        `json:"git_branch,omitempty"`
	Model              string        `json:"model,omitempty"`
	HasPlan            bool          `json:"has_plan"`
	HasDiff            bool          `json:"has_diff"`
	HasUncommittedDiff bool          `json:"has_uncommitted_diff"`
}

type sessionDetail struct {
	sessionSummary
	TotalUsage session.Usage `json:"total_usage"`
	DiffTarget string        `json:"diff_target,omitempty"`
}

type sessionsResponse struct {
	Sessions []sessionSummary `json:"sessions"`
}

type turnsResponse struct {
	Turns []*session.Turn `json:"turns"`
}

type planResponse struct {
	PlanContent  string `json:"plan_content"`
	PlanFilePath string `json:"plan_file_path,omitempty"`
}

type diffResponse struct {
	Target    string `json:"target,omitempty"`
	Diff      string `json:"diff"`
	Truncated bool   `json:"truncated"`
}

type usageResponse struct {
	TotalUsage session.Usage `json:"total_usage"`
}

type healthzResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func newSessionSummary(sess *session.Session) sessionSummary {
	return sessionSummary{
		Id:                 sess.Meta.SessionId,
		Agent:              sess.Agent,
		Title:              sess.Title,
		LastActive:         sess.LastActive,
		CWD:                sess.Meta.CWD,
		GitBranch:          sess.Meta.GitBranch,
		Model:              sess.Meta.Model,
		HasPlan:            sess.PlanContent != "" || sess.PlanFilePath != "",
		HasDiff:            sess.DiffOutput != "",
		HasUncommittedDiff: sess.UncommittedDiff != "",
	}
}
