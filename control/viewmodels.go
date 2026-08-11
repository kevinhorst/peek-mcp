package control

import (
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/tools"
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
	Total    int              `json:"total"`
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

type eventsResponse struct {
	Counters      session.Counters    `json:"counters"`
	Events        []*tools.EventEntry `json:"events"`
	PlanRevisions int                 `json:"plan_revisions"`
	Usage         session.Usage       `json:"usage"`
}

type healthzResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type Config struct {
	Transport          string `json:"transport"`
	Port               int    `json:"port"`
	Depth              int    `json:"depth"`
	ClaudeHome         string `json:"claude_home,omitempty"`
	CodexHome          string `json:"codex_home,omitempty"`
	PollInterval       string `json:"poll_interval"`
	PollWindow         string `json:"poll_window"`
	StateDir           string `json:"state_dir,omitempty"`
	StateRetentionDays int    `json:"state_retention_days"`
	ControlPort        int    `json:"control_port"`
	TokenSet           bool   `json:"token_set"`
	LogLevel           string `json:"log_level"`
}

type sessionCounts struct {
	Claude int `json:"claude"`
	Codex  int `json:"codex"`
	Total  int `json:"total"`
}

type statsResponse struct {
	PID              int            `json:"pid"`
	Version          string         `json:"version"`
	StartedAt        time.Time      `json:"started_at"`
	Uptime           string         `json:"uptime"`
	HeapAllocBytes   int64          `json:"heap_alloc_bytes"`
	SysBytes         int64          `json:"sys_bytes"`
	Goroutines       int            `json:"goroutines"`
	StateDiskBytes   int64          `json:"state_disk_bytes,omitempty"`
	Sessions         sessionCounts  `json:"sessions"`
	Invocations      map[string]int `json:"invocations,omitempty"`
	SSEClients       int64          `json:"sse_clients"`
	Config           Config         `json:"config"`
	RestartAvailable bool           `json:"restart_available"`
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
