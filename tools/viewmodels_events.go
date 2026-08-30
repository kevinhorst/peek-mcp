package tools

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/kevinhorst/peek-mcp/telemetry"
)

const maxEventSummaryChars = 200

type EventEntry struct {
	Actor     string            `json:"actor,omitempty"`
	Event     session.EventKind `json:"event"`
	Summary   string            `json:"summary,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

type planRevisionsView struct {
	Count      int         `json:"count"`
	Timestamps []time.Time `json:"timestamps,omitempty"`
}

type telemetryTimeView struct {
	ActiveSeconds int                   `json:"active_seconds,omitempty"`
	CostUSD       float64               `json:"cost_usd,omitempty"`
	Detail        string                `json:"detail,omitempty"`
	Status        telemetry.ExportState `json:"status"`
}

type sessionTimeView struct {
	StartedAt     time.Time          `json:"started_at"`
	LastActive    time.Time          `json:"last_active"`
	WallSeconds   int                `json:"wall_seconds"`
	IdleSeconds   int                `json:"idle_seconds"`
	ActiveSeconds int                `json:"active_seconds"`
	Telemetry     *telemetryTimeView `json:"telemetry,omitempty"`
}

type sessionEventsResult struct {
	Counters      *session.Counters   `json:"counters,omitempty"`
	Diff          string              `json:"diff,omitempty"`
	Events        any                 `json:"events,omitempty"`
	Permissions   *permissionsView    `json:"permissions,omitempty"`
	PlanRevisions *planRevisionsView  `json:"plan_revisions,omitempty"`
	Revisions     any                 `json:"revisions,omitempty"`
	Skills        []*skillStatView    `json:"skills,omitempty"`
	SubagentIds   []string            `json:"subagent_ids,omitempty"`
	Subagents     []*subagentStatView `json:"subagents,omitempty"`
	Time          *sessionTimeView    `json:"time,omitempty"`
	TouchedFiles  []*touchedFileView  `json:"touched_files,omitempty"`
	Unsupported   []string            `json:"unsupported,omitempty"`
	Usage         *session.Usage      `json:"usage,omitempty"`
}

type touchedFileView struct {
	Path   string `json:"path"`
	Reads  int    `json:"reads,omitempty"`
	Writes int    `json:"writes,omitempty"`
}

func newTouchedFileViews(currentSession *session.Session) []*touchedFileView {
	if len(currentSession.TouchedFiles) == 0 {
		return nil
	}

	views := make([]*touchedFileView, 0, len(currentSession.TouchedFiles))
	for path, counts := range currentSession.TouchedFiles {
		views = append(views, &touchedFileView{Path: path, Reads: counts.Reads, Writes: counts.Writes})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Path < views[j].Path })
	return views
}

type permissionsView struct {
	AutoAllowed    int                            `json:"auto_allowed"`
	HookDecided    int                            `json:"hook_decided,omitempty"`
	PromptedOnce   int                            `json:"prompted_once"`
	PromptedAlways int                            `json:"prompted_always"`
	Rejected       int                            `json:"rejected"`
	Aborted        int                            `json:"aborted,omitempty"`
	Requests       []telemetry.PermissionDecision `json:"requests,omitempty"`
	Detail         string                         `json:"detail,omitempty"`
}

func newPermissionsView(currentSession *session.Session, telemetryStore *telemetry.Store, stateDir *state.Dir) *permissionsView {
	if currentSession.Agent != session.AgentClaude {
		return nil
	}

	sessionId := string(currentSession.Meta.SessionId)
	if telemetryStore != nil {
		if stats, ok := telemetryStore.Get(sessionId); ok && !stats.Permissions.IsZero() {
			return permissionsViewFromStats(&stats.Permissions, "")
		}
	}

	if stats, ok := telemetry.ReadPersisted(stateDir, sessionId); ok && !stats.Permissions.IsZero() {
		return permissionsViewFromStats(&stats.Permissions, "persisted")
	}
	return nil
}

func permissionsViewFromStats(stats *telemetry.PermissionStats, detail string) *permissionsView {
	return &permissionsView{
		AutoAllowed:    stats.AutoAllowed,
		HookDecided:    stats.HookDecided,
		PromptedOnce:   stats.PromptedOnce,
		PromptedAlways: stats.PromptedAlways,
		Rejected:       stats.Rejected,
		Aborted:        stats.Aborted,
		Requests:       stats.Requests,
		Detail:         detail,
	}
}

func newTelemetryTimeView(currentSession *session.Session, detector *telemetry.Detector, telemetryStore *telemetry.Store, stateDir *state.Dir) *telemetryTimeView {
	if currentSession.Agent != session.AgentClaude {
		return nil
	}

	if telemetryStore != nil {
		if stats, ok := telemetryStore.Get(string(currentSession.Meta.SessionId)); ok {
			return &telemetryTimeView{
				ActiveSeconds: int(stats.ActiveSeconds),
				CostUSD:       stats.CostUSD,
				Status:        telemetry.ExportReceiving,
			}
		}
	}

	if stats, ok := telemetry.ReadPersisted(stateDir, string(currentSession.Meta.SessionId)); ok {
		return &telemetryTimeView{
			ActiveSeconds: int(stats.ActiveSeconds),
			CostUSD:       stats.CostUSD,
			Detail:        "persisted",
			Status:        telemetry.ExportReceiving,
		}
	}

	if detector == nil {
		return nil
	}
	status := detector.Status()
	return &telemetryTimeView{Detail: status.Detail, Status: status.State}
}

func newSessionTimeView(currentSession *session.Session) *sessionTimeView {
	if currentSession.StartedAt.IsZero() {
		return nil
	}

	wall := currentSession.LastActive.Sub(currentSession.StartedAt)
	idle := currentSession.Idle
	return &sessionTimeView{
		StartedAt:     currentSession.StartedAt,
		LastActive:    currentSession.LastActive,
		WallSeconds:   int(wall.Seconds()),
		IdleSeconds:   int(idle.Seconds()),
		ActiveSeconds: int((wall - idle).Seconds()),
	}
}

type sessionEventsResultPage struct {
	*sessionEventsResult
	HasMore   bool   `json:"has_more"`
	RequestId string `json:"request_id,omitempty"`
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

type subagentStatView struct {
	AgentId     string         `json:"agent_id"`
	AgentType   string         `json:"agent_type,omitempty"`
	Description string         `json:"description,omitempty"`
	StartedAt   time.Time      `json:"started_at"`
	LastActive  time.Time      `json:"last_active"`
	Seconds     int            `json:"seconds"`
	Usage       *session.Usage `json:"usage,omitempty"`
}

func newSubagentStatViews(currentSession *session.Session) []*subagentStatView {
	if len(currentSession.Subagents) == 0 {
		return nil
	}

	views := make([]*subagentStatView, 0, len(currentSession.Subagents))
	for agentId, stat := range currentSession.Subagents {
		usage := stat.Usage
		views = append(views, &subagentStatView{
			AgentId:     agentId,
			AgentType:   stat.AgentType,
			Description: stat.Description,
			StartedAt:   stat.FirstActive,
			LastActive:  stat.LastActive,
			Seconds:     int(stat.LastActive.Sub(stat.FirstActive).Seconds()),
			Usage:       &usage,
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].StartedAt.Before(views[j].StartedAt) })
	return views
}

type skillStatView struct {
	Skill     string         `json:"skill"`
	Args      string         `json:"args,omitempty"`
	StartedAt time.Time      `json:"started_at"`
	EndedAt   time.Time      `json:"ended_at"`
	Seconds   int            `json:"seconds"`
	Usage     *session.Usage `json:"usage,omitempty"`
}

func newSkillStatViews(currentSession *session.Session) []*skillStatView {
	if len(currentSession.Skills) == 0 {
		return nil
	}

	views := make([]*skillStatView, 0, len(currentSession.Skills))
	for _, stat := range currentSession.Skills {
		ended := stat.EndedAt
		if ended.IsZero() {
			ended = currentSession.LastActive
		}
		usage := stat.Usage
		views = append(views, &skillStatView{
			Skill:     stat.Skill,
			Args:      stat.Args,
			StartedAt: stat.StartedAt,
			EndedAt:   ended,
			Seconds:   int(ended.Sub(stat.StartedAt).Seconds()),
			Usage:     &usage,
		})
	}
	return views
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSpace(line)
}

func NewEventEntries(events []*session.Event) []*EventEntry {
	entries := make([]*EventEntry, 0, len(events))
	for _, event := range events {
		entry := &EventEntry{
			Actor:     event.Actor,
			Event:     event.Kind,
			Summary:   summarizeEvent(event),
			Timestamp: event.Timestamp,
		}
		entries = append(entries, entry)
	}
	return entries
}

func modelSummary(payload *session.ModelPayload) string {
	if payload == nil {
		return ""
	}

	return payload.From + " -> " + payload.To
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

func permissionModeSummary(payload *session.PermissionModePayload) string {
	if payload == nil {
		return ""
	}

	if payload.From == "" {
		return payload.To
	}

	return payload.From + " -> " + payload.To
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
	case session.EventKindModelChanged:
		summary = modelSummary(event.Model)
	case session.EventKindPermissionDenied, session.EventKindPermissionGranted:
		summary = permissionSummary(event.Permission)
	case session.EventKindPermissionModeChanged:
		summary = permissionModeSummary(event.PermissionMode)
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
