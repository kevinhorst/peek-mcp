package tools

import (
	"github.com/kevinhorst/peek-mcp/session"
)

type sessionDiffResult struct {
	CapturedAt string `json:"captured_at,omitempty"`
	Diff       string `json:"diff"`
	DiffTarget string `json:"diff_target,omitempty"`
	Source     string `json:"source"`
}

type sessionGetResult struct {
	Events     []*eventEntry      `json:"events,omitempty"`
	Memory     *memoryBlockResult `json:"memory,omitempty"`
	TotalUsage *session.Usage     `json:"total_usage,omitempty"`
	Turns      []*session.Turn    `json:"turns"`
}

type sessionLatestResult struct {
	Events []*eventEntry   `json:"events,omitempty"`
	Turns  []*session.Turn `json:"turns"`
}

type sessionPlanResult struct {
	Plan string `json:"plan"`
}

type sessionUncommittedDiffResult struct {
	Diff string `json:"diff"`
}
