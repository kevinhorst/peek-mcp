package tools

import (
	"time"

	"github.com/kevinhorst/peek-mcp/index"
	"github.com/kevinhorst/peek-mcp/session"
)

const (
	defaultSkillRuns = 20
	listArgsRunes    = 80
	listTitleRunes   = 60
	detailArgsRunes  = 500
)

// skillRunRow is one run in the list: enough to pick the comparable run, nothing to cost it.
type skillRunRow struct {
	Agents    int        `json:"agents"`
	Args      string     `json:"args,omitempty"`
	Seconds   int        `json:"seconds"`
	SessionId session.Id `json:"session_id"`
	StartedAt time.Time  `json:"started_at"`
	Title     string     `json:"title,omitempty"`
}

func newSkillRunRow(sessionRun *index.SessionRun) *skillRunRow {
	return &skillRunRow{
		Agents:    sessionRun.Run.AgentCount(),
		Args:      truncateRunes(sessionRun.Run.Args, listArgsRunes),
		Seconds:   sessionRun.Run.Seconds,
		SessionId: sessionRun.Record.SessionId,
		StartedAt: sessionRun.Run.StartedAt,
		Title:     truncateRunes(sessionRun.Record.Title, listTitleRunes),
	}
}

type skillRunsResult struct {
	Runs  []*skillRunRow `json:"runs"`
	Total int            `json:"total"`
}

// skillRunsDetail is one session's runs of the skill with every cost field.
type skillRunsDetail struct {
	Runs      []*index.Run `json:"runs"`
	SessionId session.Id   `json:"session_id"`
	Title     string       `json:"title,omitempty"`
}

func newSkillRunsDetail(sessionRuns []*index.SessionRun, sessionId session.Id) *skillRunsDetail {
	detail := &skillRunsDetail{Runs: make([]*index.Run, 0), SessionId: sessionId}
	for _, sessionRun := range sessionRuns {
		if sessionRun.Record.SessionId != sessionId {
			continue
		}

		run := *sessionRun.Run
		run.Args = truncateRunes(run.Args, detailArgsRunes)
		detail.Runs = append(detail.Runs, &run)
		detail.Title = sessionRun.Record.Title
	}
	return detail
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
