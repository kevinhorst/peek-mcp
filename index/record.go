package index

import (
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

// Root is the main agent's share of a skill run.
type Root struct {
	session.ContextLoad
	Model string        `json:"model,omitempty"`
	Usage session.Usage `json:"usage"`
}

// Run is one skill run as the index keeps it: who ran what, how long, and what it cost per agent type.
type Run struct {
	Agents    []*session.AgentTypeUsage `json:"agents,omitempty"`
	Args      string                    `json:"args,omitempty"`
	EndedAt   time.Time                 `json:"ended_at"`
	Root      *Root                     `json:"root"`
	Seconds   int                       `json:"seconds"`
	Skill     string                    `json:"skill"`
	Source    string                    `json:"source,omitempty"`
	StartedAt time.Time                 `json:"started_at"`
}

// newRun ends the run at its last main-agent or subagent activity, so the idle gap before the next invocation never counts.
func newRun(currentSession *session.Session, run *session.SkillRun) *Run {
	agents := currentSession.SkillRunAgents(run)
	endedAt := run.LastActive
	for _, agentType := range agents {
		if agentType.LastActive.After(endedAt) {
			endedAt = agentType.LastActive
		}
	}

	root := &Root{
		ContextLoad: run.ContextLoad,
		Model:       run.Model,
		Usage:       run.Usage,
	}
	return &Run{
		Agents:    agents,
		Args:      run.Args,
		EndedAt:   endedAt,
		Root:      root,
		Seconds:   int(endedAt.Sub(run.StartedAt).Seconds()),
		Skill:     run.Skill,
		Source:    run.Source,
		StartedAt: run.StartedAt,
	}
}

// AgentCount is how many subagents the run spawned.
func (r *Run) AgentCount() int {
	count := 0
	for _, agentType := range r.Agents {
		count += agentType.Count
	}
	return count
}

// Source stamps the parent transcript a record was built from; a changed stamp rebuilds it.
type Source struct {
	ModTime time.Time `json:"mod_time"`
	Size    int64     `json:"size"`
}

// Record is one session's summary: identity, times, the main agent's token spend and its skill runs.
type Record struct {
	SessionId  session.Id    `json:"session_id"`
	CWD        string        `json:"cwd,omitempty"`
	LastActive time.Time     `json:"last_active"`
	Runs       []*Run        `json:"runs,omitempty"`
	Source     Source        `json:"source"`
	StartedAt  time.Time     `json:"started_at"`
	Title      string        `json:"title,omitempty"`
	Usage      session.Usage `json:"usage"`
}

// NewRecord summarises a session; the caller holds the store lock or owns the session.
func NewRecord(currentSession *session.Session, source Source) *Record {
	runs := make([]*Run, 0, len(currentSession.SkillRuns))
	for _, run := range currentSession.SkillRuns {
		runs = append(runs, newRun(currentSession, run))
	}
	return &Record{
		SessionId:  currentSession.Meta.SessionId,
		CWD:        currentSession.Meta.CWD,
		LastActive: currentSession.LastActive,
		Runs:       runs,
		Source:     source,
		StartedAt:  currentSession.StartedAt,
		Title:      currentSession.Title,
		Usage:      currentSession.TotalUsage,
	}
}
