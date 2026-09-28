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

type TelemetryTimeView struct {
	ActiveSeconds int                   `json:"active_seconds,omitempty"`
	CostUSD       float64               `json:"cost_usd,omitempty"`
	Detail        string                `json:"detail,omitempty"`
	Status        telemetry.ExportState `json:"status"`
}

type SessionTimeView struct {
	StartedAt     time.Time          `json:"started_at"`
	LastActive    time.Time          `json:"last_active"`
	WallSeconds   int                `json:"wall_seconds"`
	IdleSeconds   int                `json:"idle_seconds"`
	ActiveSeconds int                `json:"active_seconds"`
	Telemetry     *TelemetryTimeView `json:"telemetry,omitempty"`
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
	Subagents     []*SubagentStatView `json:"subagents,omitempty"`
	Time          *SessionTimeView    `json:"time,omitempty"`
	TouchedFiles  []*touchedFileView  `json:"touched_files,omitempty"`
	Unsupported   []string            `json:"unsupported,omitempty"`
	Usage         *session.Usage      `json:"usage,omitempty"`
}

type touchedFileView struct {
	Path   string `json:"path"`
	Reads  int    `json:"reads,omitempty"`
	Writes int    `json:"writes,omitempty"`
}

func newTouchedFileViews(touchedFiles map[string]*session.FileTouchCounts) []*touchedFileView {
	if len(touchedFiles) == 0 {
		return nil
	}

	views := make([]*touchedFileView, 0, len(touchedFiles))
	for path, counts := range touchedFiles {
		views = append(views, &touchedFileView{Path: path, Reads: counts.Reads, Writes: counts.Writes})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Path < views[j].Path })
	return views
}

const (
	detailPersisted      = "persisted"
	detailTranscriptOnly = "transcript-only"
)

type deniedByKindView struct {
	AutomodeBlocked int `json:"automode_blocked"`
	PermissionRule  int `json:"permission_rule"`
	UserRejected    int `json:"user_rejected"`
}

type deniedBySourceView struct {
	Aborted int `json:"aborted"`
	Config  int `json:"config"`
	Hook    int `json:"hook"`
	Prompt  int `json:"prompt"`
}

type permissionsView struct {
	Aborted        int                            `json:"aborted,omitempty"`
	AutoAllowed    int                            `json:"auto_allowed"`
	ConfigDenied   int                            `json:"config_denied"`
	Denied         int                            `json:"denied"`
	DeniedByKind   *deniedByKindView              `json:"denied_by_kind"`
	DeniedBySource *deniedBySourceView            `json:"denied_by_source"`
	Detail         string                         `json:"detail,omitempty"`
	HookAllowed    int                            `json:"hook_allowed"`
	HookDenied     int                            `json:"hook_denied"`
	PromptedAlways int                            `json:"prompted_always"`
	PromptedOnce   int                            `json:"prompted_once"`
	Rejected       int                            `json:"rejected"`
	Requests       []telemetry.PermissionDecision `json:"requests,omitempty"`
	TelemetryOnly  int                            `json:"telemetry_only"`
	Unattributed   int                            `json:"unattributed"`
}

// newPermissionsView reconciles the transcript's denial ledger with the
// telemetry decisions: the ledger counts, telemetry attributes by tool use id.
func newPermissionsView(currentSession *session.Session, telemetryStore *telemetry.Store, stateDir *state.Dir) *permissionsView {
	if currentSession.Agent != session.AgentClaude {
		return nil
	}

	stats, detail := permissionStatsFor(currentSession, telemetryStore, stateDir)
	hasDenials := currentSession.Counters.PermissionDenials > 0
	if stats == nil && !hasDenials {
		return nil
	}

	view := permissionsViewFromStats(stats, detail)
	view.Denied = currentSession.Counters.PermissionDenials
	view.DeniedByKind = deniedByKind(currentSession.Denials)
	view.DeniedBySource, view.Unattributed, view.TelemetryOnly = reconcileDenials(currentSession.Denials, stats)
	return view
}

// permissionStatsFor returns the telemetry stats and their provenance: live,
// persisted, or none (transcript-only).
func permissionStatsFor(currentSession *session.Session, telemetryStore *telemetry.Store, stateDir *state.Dir) (*telemetry.PermissionStats, string) {
	sessionId := string(currentSession.Meta.SessionId)
	if telemetryStore != nil {
		if stats, ok := telemetryStore.Get(sessionId); ok && !stats.Permissions.IsZero() {
			return &stats.Permissions, ""
		}
	}

	if stats, ok := telemetry.ReadPersisted(stateDir, sessionId); ok && !stats.Permissions.IsZero() {
		return &stats.Permissions, detailPersisted
	}
	return nil, detailTranscriptOnly
}

func permissionsViewFromStats(stats *telemetry.PermissionStats, detail string) *permissionsView {
	view := &permissionsView{Detail: detail}
	if stats == nil {
		return view
	}

	view.Aborted = stats.Aborted
	view.AutoAllowed = stats.AutoAllowed
	view.ConfigDenied = stats.ConfigDenied
	view.HookAllowed = stats.HookAllowed
	view.HookDenied = stats.HookDenied
	view.PromptedAlways = stats.PromptedAlways
	view.PromptedOnce = stats.PromptedOnce
	view.Rejected = stats.Rejected
	view.Requests = stats.Requests
	return view
}

func countSource(view *deniedBySourceView, source string) {
	switch source {
	case "config":
		view.Config++
	case "hook":
		view.Hook++
	case "user_reject":
		view.Prompt++
	case "user_abort":
		view.Aborted++
	}
}

func deniedByKind(denials []*session.Event) *deniedByKindView {
	view := &deniedByKindView{}
	for _, denial := range denials {
		switch denial.Permission.Kind {
		case session.DenialKindAutomodeBlocked:
			view.AutomodeBlocked++
		case session.DenialKindPermissionRule:
			view.PermissionRule++
		case session.DenialKindUserRejected:
			view.UserRejected++
		}
	}
	return view
}

func isCancellationKind(kind string) bool {
	return kind == session.DenialKindCancelled || kind == session.DenialKindInterrupted
}

// countedDenialIds returns the tool use ids of the denials the counter holds,
// cancellations and id-less denials excluded.
func countedDenialIds(denials []*session.Event) map[string]bool {
	counted := make(map[string]bool, len(denials))
	for _, denial := range denials {
		hasId := denial.Permission.ToolUseId != ""
		if hasId && !isCancellationKind(denial.Permission.Kind) {
			counted[denial.Permission.ToolUseId] = true
		}
	}
	return counted
}

// reconcileDenials joins the transcript ledger to telemetry's listed rejects
// by tool use id: attributed per source, unattributed (no decision event, the
// unresolved asks), and telemetry-only (a reject with no transcript denial).
func reconcileDenials(denials []*session.Event, stats *telemetry.PermissionStats) (*deniedBySourceView, int, int) {
	counted := countedDenialIds(denials)
	bySource := &deniedBySourceView{}
	attributed := make(map[string]bool)
	telemetryOnly := 0
	if stats != nil {
		for index := range stats.Requests {
			request := &stats.Requests[index]
			if request.Decision != "reject" {
				continue
			}
			if !counted[request.ToolUseId] {
				telemetryOnly++
				continue
			}
			attributed[request.ToolUseId] = true
			countSource(bySource, request.Source)
		}
	}

	unattributed := 0
	for _, denial := range denials {
		if isCancellationKind(denial.Permission.Kind) || attributed[denial.Permission.ToolUseId] {
			continue
		}
		unattributed++
	}
	return bySource, unattributed, telemetryOnly
}

func NewTelemetryTimeView(currentSession *session.Session, detector *telemetry.Detector, telemetryStore *telemetry.Store, stateDir *state.Dir) *TelemetryTimeView {
	if currentSession.Agent != session.AgentClaude {
		return nil
	}

	if telemetryStore != nil {
		if stats, ok := telemetryStore.Get(string(currentSession.Meta.SessionId)); ok {
			return &TelemetryTimeView{
				ActiveSeconds: int(stats.ActiveSeconds),
				CostUSD:       stats.CostUSD,
				Status:        telemetry.ExportReceiving,
			}
		}
	}

	if stats, ok := telemetry.ReadPersisted(stateDir, string(currentSession.Meta.SessionId)); ok {
		return &TelemetryTimeView{
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
	return &TelemetryTimeView{Detail: status.Detail, Status: status.State}
}

func NewSessionTimeView(currentSession *session.Session) *SessionTimeView {
	if currentSession.StartedAt.IsZero() {
		return nil
	}

	wall := currentSession.LastActive.Sub(currentSession.StartedAt)
	idle := currentSession.Idle
	return &SessionTimeView{
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

type SubagentStatView struct {
	AgentId      string             `json:"agent_id"`
	AgentType    string             `json:"agent_type,omitempty"`
	Description  string             `json:"description,omitempty"`
	Model        string             `json:"model,omitempty"`
	StartedAt    time.Time          `json:"started_at"`
	LastActive   time.Time          `json:"last_active"`
	Seconds      int                `json:"seconds"`
	TouchedFiles []*touchedFileView `json:"touched_files,omitempty"`
	Usage        *session.Usage     `json:"usage,omitempty"`
}

func NewSubagentStatViews(currentSession *session.Session) []*SubagentStatView {
	if len(currentSession.Subagents) == 0 {
		return nil
	}

	views := make([]*SubagentStatView, 0, len(currentSession.Subagents))
	for agentId, stat := range currentSession.Subagents {
		usage := stat.Usage
		views = append(views, &SubagentStatView{
			AgentId:      agentId,
			AgentType:    stat.AgentType,
			Description:  stat.Description,
			Model:        stat.Model,
			StartedAt:    stat.FirstActive,
			LastActive:   stat.LastActive,
			Seconds:      int(stat.LastActive.Sub(stat.FirstActive).Seconds()),
			TouchedFiles: newTouchedFileViews(stat.TouchedFiles),
			Usage:        &usage,
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].StartedAt.Before(views[j].StartedAt) })
	return views
}

type skillStatView struct {
	Agent     string         `json:"agent,omitempty"`
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
			Agent:     stat.AgentId,
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

	summary := payload.Tool
	if payload.Command != "" {
		summary += ": " + payload.Command
	}
	if payload.Kind == "" {
		return summary
	}
	return payload.Kind + " " + summary
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
	case session.EventKindTaskCompleted:
		summary = taskSummary(event.Task)
	case session.EventKindUserAnswer:
		summary = userAnswerSummary(event.UserAnswer)
	}
	return truncateSummary(summary)
}

func taskSummary(payload *session.TaskPayload) string {
	if payload == nil {
		return ""
	}

	return payload.TaskId + " " + payload.Status + ": " + payload.Summary
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
