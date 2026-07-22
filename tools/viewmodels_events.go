package tools

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

const maxEventSummaryChars = 200

type eventEntry struct {
	Actor     string            `json:"actor,omitempty"`
	Event     session.EventKind `json:"event"`
	Summary   string            `json:"summary,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

type planRevisionsView struct {
	Count      int         `json:"count"`
	Timestamps []time.Time `json:"timestamps,omitempty"`
}

type sessionEventsResult struct {
	Counters      *session.Counters  `json:"counters,omitempty"`
	Diff          string             `json:"diff,omitempty"`
	Events        json.RawMessage    `json:"events,omitempty"`
	PlanRevisions *planRevisionsView `json:"plan_revisions,omitempty"`
	Revisions     json.RawMessage    `json:"revisions,omitempty"`
	Unsupported   []string           `json:"unsupported,omitempty"`
	Usage         *session.Usage     `json:"usage,omitempty"`
}

type sessionEventsResultPage struct {
	*sessionEventsResult
	RequestId string `json:"request_id,omitempty"`
	HasMore   bool   `json:"has_more"`
}

func newSessionEventsResultPage(result *sessionEventsResult) *sessionEventsResultPage {
	return &sessionEventsResultPage{
		sessionEventsResult: result,
	}
}

func (p *sessionEventsResultPage) WithRequestId(id string) {
	p.HasMore = true
	p.RequestId = id
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSpace(line)
}

func newEventEntries(events []*session.Event) []*eventEntry {
	entries := make([]*eventEntry, 0, len(events))
	for _, event := range events {
		entry := &eventEntry{
			Actor:     event.Actor,
			Event:     event.Kind,
			Summary:   summarizeEvent(event),
			Timestamp: event.Timestamp,
		}
		entries = append(entries, entry)
	}
	return entries
}

func permissionSummary(payload *session.PermissionPayload) string {
	if payload == nil {
		return ""
	}

	if payload.Command == "" {
		return payload.Tool
	}

	return payload.Tool + ": " + payload.Command
}

func planRevisionSummary(payload *session.PlanPayload) string {
	if payload == nil {
		return ""
	}

	return "revision " + strconv.Itoa(payload.Revision)
}

func skillSummary(payload *session.SkillPayload) string {
	if payload == nil {
		return ""
	}

	if payload.Args == "" {
		return payload.Skill
	}

	return payload.Skill + " " + payload.Args
}

func subagentSummary(payload *session.SubagentPayload) string {
	if payload == nil {
		return ""
	}

	if payload.Description != "" {
		return payload.AgentType + ": " + payload.Description
	}

	return firstLine(payload.Content)
}

func summarizeEvent(event *session.Event) string {
	summary := ""
	switch event.Kind {
	case session.EventKindPermissionDenied:
		summary = permissionSummary(event.Permission)
	case session.EventKindPlanApproved, session.EventKindPlanModeEnter,
		session.EventKindPlanModeExit, session.EventKindPlanModeReenter,
		session.EventKindPlanRejected:
	case session.EventKindPlanRevised:
		summary = planRevisionSummary(event.Plan)
	case session.EventKindSkillInvoked:
		summary = skillSummary(event.Skill)
	case session.EventKindSubagentResult, session.EventKindSubagentSpawned:
		summary = subagentSummary(event.Subagent)
	case session.EventKindUserAnswer:
		summary = userAnswerSummary(event.UserAnswer)
	}
	return truncateSummary(summary)
}

func truncateSummary(summary string) string {
	runes := []rune(summary)
	if len(runes) <= maxEventSummaryChars {
		return summary
	}

	return string(runes[:maxEventSummaryChars])
}

func userAnswerSummary(payload *session.UserAnswerPayload) string {
	if payload == nil {
		return ""
	}

	return firstLine(payload.Answers)
}
