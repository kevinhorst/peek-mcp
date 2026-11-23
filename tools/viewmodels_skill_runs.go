package tools

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

const defaultSkillRuns = 5

// skillNamespaceSeparator joins a plugin's name and its skill's name, as in "plugin:skill".
const skillNamespaceSeparator = ":"

type skillRunRootView struct {
	session.ContextLoad
	Model string         `json:"model,omitempty"`
	Usage *session.Usage `json:"usage"`
}

type skillRunView struct {
	Agents    []*session.AgentTypeUsage `json:"agents,omitempty"`
	Args      string                    `json:"args,omitempty"`
	CWD       string                    `json:"cwd,omitempty"`
	EndedAt   time.Time                 `json:"ended_at"`
	Root      *skillRunRootView         `json:"root"`
	Seconds   int                       `json:"seconds"`
	SessionId session.Id                `json:"session_id"`
	Skill     string                    `json:"skill"`
	Source    string                    `json:"source,omitempty"`
	StartedAt time.Time                 `json:"started_at"`
	Title     string                    `json:"title,omitempty"`
}

// newSkillRunView ends the run at its last root or subagent activity, so the idle gap before the next invocation never counts.
func newSkillRunView(currentSession *session.Session, run *session.SkillRun) *skillRunView {
	agents := currentSession.SkillRunAgents(run)
	endedAt := run.LastActive
	for _, agentType := range agents {
		if agentType.LastActive.After(endedAt) {
			endedAt = agentType.LastActive
		}
	}

	usage := run.Usage
	root := &skillRunRootView{
		ContextLoad: run.ContextLoad,
		Model:       run.Model,
		Usage:       &usage,
	}
	return &skillRunView{
		Agents:    agents,
		Args:      run.Args,
		CWD:       currentSession.Meta.CWD,
		EndedAt:   endedAt,
		Root:      root,
		Seconds:   int(endedAt.Sub(run.StartedAt).Seconds()),
		SessionId: currentSession.Meta.SessionId,
		Skill:     run.Skill,
		Source:    run.Source,
		StartedAt: run.StartedAt,
		Title:     currentSession.Title,
	}
}

type skillRunsResult struct {
	Runs  []*skillRunView `json:"runs"`
	Total int             `json:"total"`
}

func compareSkillRunsNewestFirst(left, right *skillRunView) int {
	return cmp.Compare(right.StartedAt.UnixNano(), left.StartedAt.UnixNano())
}

// isSkillMatch accepts the skill's own name and its plugin-namespaced form.
func isSkillMatch(invoked, wanted string) bool {
	if invoked == wanted {
		return true
	}
	return strings.HasSuffix(invoked, skillNamespaceSeparator+wanted)
}

// newSkillRunViews lists every run of the skill across the sessions, newest first.
func newSkillRunViews(sessions []*session.Session, skill string) []*skillRunView {
	views := make([]*skillRunView, 0)
	for _, currentSession := range sessions {
		for _, run := range currentSession.SkillRuns {
			if !isSkillMatch(run.Skill, skill) {
				continue
			}

			views = append(views, newSkillRunView(currentSession, run))
		}
	}
	slices.SortFunc(views, compareSkillRunsNewestFirst)
	return views
}
