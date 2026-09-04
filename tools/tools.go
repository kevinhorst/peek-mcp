package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/telemetry"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var errSessionSelectorMissing = errors.New("id or title parameter is required")

const (
	DefaultReturnedTurns = 20
)

func withMaxResultSize() *mcp.Meta {
	return mcp.NewMetaFromMap(map[string]any{
		"anthropic/maxResultSizeChars": 500_000,
	})
}

func counted(counter *InvocationCounter, name string, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := handler(ctx, req)
		counter.Inc(name, resultBytes(result))
		return result, err
	}
}

func resultBytes(result *mcp.CallToolResult) int64 {
	if result == nil {
		return 0
	}
	data, err := json.Marshal(result)
	if err != nil {
		return 0
	}
	return int64(len(data))
}

func Register(server *server.MCPServer, store *session.Store, counter *InvocationCounter, telemetryStore *telemetry.Store, detector *telemetry.Detector) {
	pageStore := &PageStore[*sessionGetResult]{
		PagesByRequestId: make(map[string]<-chan *sessionGetResult),
	}
	eventsPageStore := &PageStore[*sessionEventsResult]{
		PagesByRequestId: make(map[string]<-chan *sessionEventsResult),
	}

	sessionGet := mcp.NewTool("session_get",
		mcp.WithDescription("Returns session data (turns, events, plan, git diff, uncommitted diff, auto-memory) for a session. Defaults to the most recently active session when id and title are omitted. Select sections with the turns/events/plan/diff/uncommitted_diff/remember flags. Responses are paginated: if has_more is true, call again with the returned request_id to get the next page."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("id",
			mcp.Description("Session ID (omit for most recent session)"),
		),
		mcp.WithString("title",
			mcp.Description("Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to agent when provided. For Codex, titles come from Codex's session index (thread name)."),
		),
		mcp.WithString("agent",
			mcp.Description("Agent: \"claude\" or \"codex\". Required when id and title are omitted and more than one agent is enabled."),
		),
		mcp.WithNumber("n",
			mcp.Description("Number of turns to return (default 20). Only applies to the turns section."),
		),
		mcp.WithBoolean("turns",
			mcp.Description("Return the session turns (default true)."),
		),
		mcp.WithBoolean("events",
			mcp.Description("Return the typed event stream entries (default true)."),
		),
		mcp.WithBoolean("plan",
			mcp.Description("Return the session plan (default true)."),
		),
		mcp.WithBoolean("diff",
			mcp.Description("Return the pre-computed merge-base git diff against the inferred base branch, reported as diff_target (default true)."),
		),
		mcp.WithBoolean("uncommitted_diff",
			mcp.Description("Return the live uncommitted git diff (`git diff HEAD`) in the session's own working tree (default false)."),
		),
		mcp.WithBoolean("remember",
			mcp.Description("Include the project's auto-memory (MEMORY.md + fact files). Claude sessions only (default false)."),
		),
		mcp.WithString("subagent",
			mcp.Description("Subagent id: scope the response to that agent's transcript and events (plan/diff/memory are omitted). Valid ids are listed in every response's subagents field."),
		),
		mcp.WithBoolean("thinking",
			mcp.Description("Include assistant thinking text on turns (default false)."),
		),
		mcp.WithString("request_id",
			mcp.Description("Pagination request ID from a previous response. Pass this to get the next page."),
		),
		mcp.WithBoolean("json",
			mcp.Description("Return the full typed response as structuredContent, unpaginated — sections are real JSON objects instead of chunked strings (default false: paginated JSON text block)"),
		),
	)
	sessionGet.Meta = withMaxResultSize()
	server.AddTool(sessionGet, counted(counter, "session_get", sessionGetHandler(store, pageStore)))

	sessionList :=
		mcp.NewTool("session_list",
			mcp.WithDescription("Lists all sessions. Returns session ID, agent, last activity timestamp, whether a plan or diff is available, the inferred diff base branch (diff_target), and session metadata (cwd, git branch, model, project label, origin)."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("agent",
				mcp.Description("Agent: \"claude\" or \"codex\". Lists all sessions when omitted."),
			),
			mcp.WithString("project",
				mcp.Description("Exact project label filter (e.g. \"cowork\"). Lists all sessions when omitted."),
			),
		)
	sessionList.Meta = withMaxResultSize()
	server.AddTool(sessionList, counted(counter, "session_list", sessionListHandler(store)))

	sessionEvents := mcp.NewTool("session_events",
		mcp.WithDescription("Returns the typed event stream of a session (plan lifecycle, permission denials/grants, permission-mode changes, skill invocations, subagent spawns/results, user answers) plus derived counters, telemetry-based permission decisions (auto-allowed vs. prompted vs. rejected, with the prompted commands), token usage totals, session time (wall/idle/active seconds), touched files, plan revision history, and diff availability (live | snapshot | none). Turns are not included — use session_get for those."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("id",
			mcp.Description("Session ID (omit for most recent session)"),
		),
		mcp.WithString("title",
			mcp.Description("Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to agent when provided. For Codex, titles come from Codex's session index (thread name)."),
		),
		mcp.WithString("agent",
			mcp.Description("Agent: \"claude\" or \"codex\". Required when id and title are omitted."),
		),
		mcp.WithBoolean("revisions",
			mcp.Description("Include plan revision diffs (default false; they dominate response size)"),
		),
		mcp.WithBoolean("breakdown",
			mcp.Description("Include per-skill and per-subagent time and token usage (default false; Claude sessions only)"),
		),
		mcp.WithString("subagent",
			mcp.Description("Subagent id: scope events and breakdown to that agent. Valid ids are listed in every response's subagent_ids field."),
		),
		mcp.WithString("request_id",
			mcp.Description("Pagination request ID from a previous response. Pass this to get the next page."),
		),
		mcp.WithBoolean("json",
			mcp.Description("Return the full typed response as structuredContent, unpaginated — sections are real JSON objects instead of chunked strings (default false: paginated JSON text block)"),
		),
	)
	sessionEvents.Meta = withMaxResultSize()
	server.AddTool(sessionEvents, counted(counter, "session_events", sessionEventsHandler(detector, store, eventsPageStore, telemetryStore)))
}

func sessionGetHandler(s *session.Store, pageStore *PageStore[*sessionGetResult]) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		if reqId, ok := args["request_id"].(string); ok && reqId != "" {
			next, ok := pageStore.next(reqId)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("request_id %q not found or expired", reqId)), nil
			}

			if !pageStore.hasNext(reqId) {
				pageStore.remove(reqId)
				reqId = ""
			}

			result := &sessionGetResultPage{
				sessionGetResult: next,
				RequestId:        reqId,
				HasMore:          pageStore.hasNext(reqId),
			}
			return respond(request, result)
		}

		sess, err := resolveSession(s, request)
		if err != nil {
			if !errors.Is(err, errSessionSelectorMissing) {
				return mcp.NewToolResultError(err.Error()), nil
			}
			agent, agentErr := resolveAgentFromRequest(s, request)
			if agentErr != nil {
				return mcp.NewToolResultError(agentErr.Error()), nil
			}
			found, ok := s.Last(agent)
			if !ok {
				return mcp.NewToolResultText("no sessions found"), nil
			}
			sess = found
		}

		withTurns := boolArgFromRequest(request, "turns", true)
		withEvents := boolArgFromRequest(request, "events", true)
		withPlan := boolArgFromRequest(request, "plan", true)
		withDiff := boolArgFromRequest(request, "diff", true)
		withUncommitted := boolArgFromRequest(request, "uncommitted_diff", false)
		withMemory := boolArgFromRequest(request, "remember", false)
		n := intArgFromRequest(request, "n")
		if n <= 0 {
			n = DefaultReturnedTurns
		}

		subagentId, _ := args["subagent"].(string)
		withThinking := boolArgFromRequest(request, "thinking", false)

		scopedTurns := sess.Turns(n)
		scopedEvents := sess.Events.All()
		if subagentId != "" {
			subagentTurns, ok := sess.SubagentTurns(subagentId, n)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("unknown subagent id %q; valid ids: %v", subagentId, sess.SubagentIds())), nil
			}
			scopedTurns = subagentTurns
			scopedEvents = filterEventsByActor(scopedEvents, subagentId)
			withPlan, withDiff, withUncommitted, withMemory = false, false, false, false
		}
		scopedTurns = turnsForOutput(scopedTurns, withThinking)

		if boolArgFromRequest(request, "json", false) {
			result := &sessionGetResult{TotalUsage: sess.CurrentUsage(), Subagents: newSubagentRefs(sess)}
			if withTurns {
				if len(scopedTurns) > 0 {
					result.Turns = scopedTurns
				}
			}
			if withEvents {
				if entries := NewEventEntries(scopedEvents); len(entries) > 0 {
					result.Events = entries
				}
			}
			if withPlan {
				result.Plan = sess.PlanContent
			}
			if withDiff {
				diffContent, diffTarget, _ := s.LoadDiff(sess.Meta.SessionId)
				result.Diff = diffContent
				result.DiffTarget = diffTarget
			}
			if withUncommitted {
				result.UncommittedDiff = sess.UncommittedDiff
			}
			if withMemory {
				result.Memory = memoryBlock(sess)
			}
			return respondWithStructured(result)
		}

		var turns, events, plan, diff, uncommitted, memory string
		if withTurns {
			data, err := json.Marshal(scopedTurns)
			if err != nil {
				return nil, fmt.Errorf("marshaling turns: %w", err)
			}
			turns = string(data)
		}
		if withEvents {
			events = marshalEventEntries(scopedEvents)
		}
		if withPlan {
			plan = sess.PlanContent
		}
		if withDiff {
			diff, _, _ = s.LoadDiff(sess.Meta.SessionId)
		}
		if withUncommitted {
			uncommitted = sess.UncommittedDiff
		}
		if withMemory {
			memory = marshalMemoryBlock(sess)
		}

		firstPage, nextPages := NewPageBuilder(maxResponseBytes(ctx)).build(
			diff,
			events,
			memory,
			plan,
			turns,
			uncommitted,
		)
		if withDiff {
			firstPage.DiffTarget = sess.DiffTarget
		}
		firstPage.TotalUsage = sess.CurrentUsage()
		firstPage.Subagents = newSubagentRefs(sess)

		resultPage := newSessionGetResultPage(firstPage)
		if len(nextPages) == 0 {
			return respond(request, resultPage)
		}

		requestId := uuid.NewString()
		pageStore.add(requestId, nextPages)

		resultPage.WithRequestId(requestId)
		return respond(request, resultPage)
	}
}

func sessionListHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agent, err := resolveAgentFromRequest(s, request)

		var sessions []*session.Session
		if err != nil {
			sessions = s.List()
		} else {
			sessions = s.List(agent)
		}
		if project := request.GetString("project", ""); project != "" {
			filtered := sessions[:0:0]
			for _, sess := range sessions {
				if sess.Meta.Project == project {
					filtered = append(filtered, sess)
				}
			}
			sessions = filtered
		}
		items := make([]sessionListItem, len(sessions))
		for i, sess := range sessions {
			items[i] = sessionListItem{
				Id:          sess.Meta.SessionId,
				Agent:       sess.Agent,
				Title:       sess.Title,
				TitleSource: sess.TitleSource,
				LastActive:  sess.LastActive,
				HasPlan:     sess.PlanContent != "" || sess.PlanFilePath != "",
				HasDiff:     sess.DiffOutput != "" || sess.HasDiffSnapshot,
				DiffTarget:  sess.DiffTarget,
				Meta:        sess.Meta,
			}
		}

		return respondWithStructured(map[string]any{"sessions": items})
	}
}

func sessionEventsHandler(detector *telemetry.Detector, s *session.Store, pageStore *PageStore[*sessionEventsResult], telemetryStore *telemetry.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		if reqId, ok := args["request_id"].(string); ok && reqId != "" {
			next, ok := pageStore.next(reqId)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("request_id %q not found or expired", reqId)), nil
			}

			if !pageStore.hasNext(reqId) {
				pageStore.remove(reqId)
				reqId = ""
			}

			result := &sessionEventsResultPage{
				sessionEventsResult: next,
				RequestId:           reqId,
				HasMore:             pageStore.hasNext(reqId),
			}
			return respond(request, result)
		}

		currentSession, err := resolveSession(s, request)
		if err != nil {
			if !errors.Is(err, errSessionSelectorMissing) {
				return mcp.NewToolResultError(err.Error()), nil
			}
			agent, agentErr := resolveAgentFromRequest(s, request)
			if agentErr != nil {
				return mcp.NewToolResultError(agentErr.Error()), nil
			}
			found, ok := s.Last(agent)
			if !ok {
				return respondWithText("No sessions found.")
			}
			currentSession = found
		}

		useJson := boolArgFromRequest(request, "json", false)
		withRevisions := boolArgFromRequest(request, "revisions", false)

		subagentId, _ := args["subagent"].(string)
		scopedEvents := currentSession.Events.All()
		if subagentId != "" {
			if _, ok := currentSession.Subagents[subagentId]; !ok {
				return mcp.NewToolResultError(fmt.Sprintf("unknown subagent id %q; valid ids: %v", subagentId, currentSession.SubagentIds())), nil
			}
			scopedEvents = filterEventsByActor(scopedEvents, subagentId)
		}

		var firstPage *sessionEventsResult
		var nextPages []*sessionEventsResult
		if useJson {
			firstPage = &sessionEventsResult{}
			if len(scopedEvents) > 0 {
				firstPage.Events = scopedEvents
			}
			if withRevisions && len(currentSession.PlanRevisions) > 0 {
				firstPage.Revisions = currentSession.PlanRevisions
			}
		} else {
			events := marshalEvents(scopedEvents)
			revisions := ""
			if withRevisions {
				revisions = marshalPlanRevisions(currentSession)
			}
			firstPage, nextPages = NewPageBuilder(maxResponseBytes(ctx)).buildEvents(
				events,
				revisions,
			)
		}
		counters := currentSession.Counters
		firstPage.Counters = &counters
		firstPage.Diff = diffAvailability(currentSession)
		firstPage.PlanRevisions = newPlanRevisionsView(currentSession)
		firstPage.Time = newSessionTimeView(currentSession)
		if firstPage.Time != nil {
			firstPage.Time.Telemetry = newTelemetryTimeView(currentSession, detector, telemetryStore, s.StateDir)
		}
		firstPage.Permissions = newPermissionsView(currentSession, telemetryStore, s.StateDir)
		firstPage.TouchedFiles = newTouchedFileViews(currentSession)
		if boolArgFromRequest(request, "breakdown", false) {
			firstPage.Skills = newSkillStatViews(currentSession)
			firstPage.Subagents = newSubagentStatViews(currentSession)
			if subagentId != "" {
				firstPage.Skills = nil
				stats := firstPage.Subagents[:0]
				for _, stat := range firstPage.Subagents {
					if stat.AgentId == subagentId {
						stats = append(stats, stat)
					}
				}
				firstPage.Subagents = stats
			}
		}
		firstPage.SubagentIds = currentSession.SubagentIds()
		firstPage.Unsupported = unsupportedSignals(currentSession.Agent)
		firstPage.Usage = currentSession.CurrentUsage()

		if useJson {
			return respondWithStructured(firstPage)
		}

		resultPage := newSessionEventsResultPage(firstPage)
		if len(nextPages) == 0 {
			return respond(request, resultPage)
		}

		requestId := uuid.NewString()
		pageStore.add(requestId, nextPages)

		resultPage.WithRequestId(requestId)
		return respond(request, resultPage)
	}
}

func diffAvailability(currentSession *session.Session) string {
	if currentSession.DiffOutput == "" && !currentSession.HasDiffSnapshot {
		return "none"
	}

	if currentSession.DiffSource == session.DiffSourceSnapshot {
		return "snapshot"
	}

	return "live"
}

func memoryBlock(currentSession *session.Session) *memoryBlockResult {
	if currentSession.Agent != session.AgentClaude {
		return &memoryBlockResult{Unsupported: "memory is not available for codex sessions"}
	}
	if currentSession.FilePath == "" {
		return &memoryBlockResult{Unsupported: "transcript path unknown"}
	}

	memory, err := claude.ReadMemory(currentSession.FilePath)
	if err != nil {
		return &memoryBlockResult{Unsupported: err.Error()}
	}

	block := &memoryBlockResult{
		Facts:       memory.Facts,
		Index:       memory.Index,
		IsTruncated: memory.IsTruncated,
	}
	return block
}

func newPlanRevisionsView(currentSession *session.Session) *planRevisionsView {
	if len(currentSession.PlanRevisions) == 0 {
		return nil
	}

	view := &planRevisionsView{Count: len(currentSession.PlanRevisions)}
	for _, revision := range currentSession.PlanRevisions {
		view.Timestamps = append(view.Timestamps, revision.Timestamp)
	}
	return view
}

func unsupportedSignals(agent session.Agent) []string {
	if agent == session.AgentCodex {
		return []string{"skills", "memory", "user_answers", "plan_approval", "subagent_results", "touched_files", "skill_usage", "subagent_usage", "telemetry", "permissions"}
	}
	return nil
}

func turnsForOutput(turns []*session.Turn, withThinking bool) []*session.Turn {
	if withThinking {
		return turns
	}

	stripped := make([]*session.Turn, len(turns))
	for i, turn := range turns {
		copied := *turn
		copied.Thinking = ""
		stripped[i] = &copied
	}
	return stripped
}

func filterEventsByActor(all []*session.Event, actor string) []*session.Event {
	filtered := make([]*session.Event, 0)
	for _, event := range all {
		if event.Actor == actor {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func marshalEvents(events []*session.Event) string {
	if len(events) == 0 {
		return ""
	}

	data, err := json.Marshal(events)
	if err != nil {
		return ""
	}
	return string(data)
}

func marshalPlanRevisions(currentSession *session.Session) string {
	if len(currentSession.PlanRevisions) == 0 {
		return ""
	}

	data, err := json.Marshal(currentSession.PlanRevisions)
	if err != nil {
		return ""
	}
	return string(data)
}

func marshalEventEntries(events []*session.Event) string {
	entries := NewEventEntries(events)
	if len(entries) == 0 {
		return ""
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return ""
	}
	return string(data)
}

func marshalMemoryBlock(currentSession *session.Session) string {
	data, err := json.Marshal(memoryBlock(currentSession))
	if err != nil {
		return ""
	}
	return string(data)
}

// resolveSession looks up a session by id or title from request args.
// Precedence: id > title.
func resolveSession(s *session.Store, request mcp.CallToolRequest) (*session.Session, error) {
	args := request.GetArguments()

	if id, ok := args["id"].(string); ok && id != "" {
		sess, found := s.GetById(session.Id(id))
		if !found {
			return nil, fmt.Errorf("session %q not found", id)
		}
		return sess, nil
	}

	if title, ok := args["title"].(string); ok && title != "" {
		agent, err := resolveAgentFilter(s, request)
		if err != nil {
			return nil, err
		}
		return s.GetByTitle(title, agent)
	}

	return nil, errSessionSelectorMissing
}

func resolveAgentFilter(s *session.Store, request mcp.CallToolRequest) (session.Agent, error) {
	raw, _ := request.GetArguments()["agent"].(string)
	if raw == "" {
		return "", nil
	}

	return s.ResolveAgent(session.Agent(raw))
}

func resolveAgentFromRequest(s *session.Store, request mcp.CallToolRequest) (session.Agent, error) {
	args := request.GetArguments()
	raw, _ := args["agent"].(string)

	return s.ResolveAgent(session.Agent(raw))
}
