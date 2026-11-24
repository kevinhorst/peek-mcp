package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/index"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/telemetry"
	"github.com/kevinhorst/peek-mcp/watcher"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var (
	errInitialLoadPending     = errors.New("initial session load still in progress; retry shortly")
	errSessionSelectorMissing = errors.New("id or title parameter is required")
	errIndexUnavailable       = errors.New("the session index needs a claude home and a state dir")
	errSkillMissing           = errors.New("skill parameter is required")
	errSkillRunsLimitInvalid  = errors.New("limit must be at least 1")
	errSkillRunsByInvalid     = errors.New(`by must be "agent_type"`)
	errWindowDaysInvalid      = errors.New("window_days must be at least 1")
)

const (
	DefaultReturnedTurns = 20
	readyTimeout         = 2 * time.Minute
	windowDaysParam      = "window_days"
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

func awaitReady(
	handler server.ToolHandlerFunc,
	lifecycle *Lifecycle,
	store *session.Store,
	timeout time.Duration,
) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		lifecycle.Acquire()
		defer lifecycle.Release()

		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case <-store.Ready():
			return handler(ctx, request)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return mcp.NewToolResultError(errInitialLoadPending.Error()), nil
		}
	}
}

// widened applies the window_days argument before the call waits for the store.
func widened(handler server.ToolHandlerFunc, lifecycle *Lifecycle) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if _, isSet := request.GetArguments()[windowDaysParam]; !isSet {
			return handler(ctx, request)
		}

		// window_days
		days := request.GetInt(windowDaysParam, 0)
		if days < 1 {
			return mcp.NewToolResultError(errWindowDaysInvalid.Error()), nil
		}

		lifecycle.Widen(time.Duration(days) * 24 * time.Hour)
		return handler(ctx, request)
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

func Register(
	counter *InvocationCounter,
	detector *telemetry.Detector,
	differ *watcher.Differ,
	indexer *index.Indexer,
	lifecycle *Lifecycle,
	loader *watcher.TurnLoader,
	server *server.MCPServer,
	store *session.Store,
	telemetryStore *telemetry.Store,
) {
	pageStore := &PageStore[*sessionGetResult]{
		PagesByRequestId: make(map[string]<-chan *sessionGetResult),
	}
	eventsPageStore := &PageStore[*sessionEventsResult]{
		PagesByRequestId: make(map[string]<-chan *sessionEventsResult),
	}

	sessionGet := mcp.NewTool("session_get",
		mcp.WithDescription("Returns session data (turns, events, plan, git diff, uncommitted diff, auto-memory) for a session. Defaults to the most recently active session when id and title are omitted. Select sections with the turns/events/plan/diff/uncommitted_diff/remember flags; diff and uncommitted_diff are off by default and dominate response size, so request them only when the code change itself is needed. Responses are paginated: if has_more is true, call again with the returned request_id to get the next page."),
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
			mcp.Description("Number of turns to return (default 20). Only applies to the turns section. Turns beyond the instance's in-memory buffer are re-read from the transcript, which is slower."),
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
			mcp.Description("Return the merge-base git diff against the inferred base branch, computed on request and reported as diff_target; once the working tree is gone, the last computed diff (default false)."),
		),
		mcp.WithBoolean("uncommitted_diff",
			mcp.Description("Return the uncommitted git diff (`git diff HEAD`) in the session's own working tree, computed on request (default false)."),
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
		mcp.WithBoolean("tools",
			mcp.Description("Include each turn's tool calls as tool_calls [{id, name, input, is_error, timestamp}]; tool-only turns are returned too and count toward n (default false). Claude sessions only."),
		),
		mcp.WithString("request_id",
			mcp.Description("Pagination request ID from a previous response. Pass this to get the next page."),
		),
		mcp.WithBoolean("json",
			mcp.Description("Return the full typed response as structuredContent, unpaginated — sections are real JSON objects instead of chunked strings (default false: paginated JSON text block)"),
		),
	)
	sessionGet.Meta = withMaxResultSize()
	getHandler := awaitReady(sessionGetHandler(differ, loader, pageStore, store), lifecycle, store, readyTimeout)
	server.AddTool(sessionGet, counted(counter, "session_get", getHandler))

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
			mcp.WithNumber(windowDaysParam,
				mcp.Description("Widen this instance's watch window to this many days before listing (stdio instances start with 3). The wider window stays until the instance exits, and the call waits while the added days load. Omit to list the current window."),
			),
		)
	sessionList.Meta = withMaxResultSize()
	listHandler := awaitReady(sessionListHandler(store), lifecycle, store, readyTimeout)
	server.AddTool(sessionList, counted(counter, "session_list", widened(listHandler, lifecycle)))

	sessionEvents := mcp.NewTool("session_events",
		mcp.WithDescription("Returns the typed event stream of a session (plan lifecycle, permission denials/grants, permission-mode changes, skill invocations, subagent spawns/results, background task completions, user answers) plus derived counters, telemetry-based permission decisions (auto-allowed vs. prompted vs. rejected, with the prompted commands), token usage totals, session time (wall/idle/active seconds), touched files, plan revision history, and diff availability (live | snapshot | none). Turns are not included — use session_get for those."),
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
		mcp.WithBoolean("details",
			mcp.Description("Include the touched-file lists (session and, with breakdown, per subagent) and the permission request list (default false: counters only; these lists dominate response size)"),
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
	eventsHandler := sessionEventsHandler(detector, store, eventsPageStore, telemetryStore)
	server.AddTool(sessionEvents, counted(counter, "session_events", awaitReady(eventsHandler, lifecycle, store, readyTimeout)))

	skillRuns := mcp.NewTool("skill_runs",
		mcp.WithDescription("A skill's runs of the last 14 days from peek's session index, never a transcript read. Without session_id: one short row per run, newest first — session_id, title, started_at, seconds, agent count, args cut to 80 characters — and total. With session_id: that session's runs of the skill with their cost — the main agent's model, requests, peak_context (the largest context one request sent) and token usage, and the subagents summed per agent type and model. A run is one invocation by the main agent and lasts until its next skill invocation. Claude sessions only."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("skill",
			mcp.Required(),
			mcp.Description("Skill name as invoked, without the slash (e.g. \"fdesign\"); a plugin-namespaced invocation (\"plugin:fdesign\") matches too."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Rows in the list (default 20); total reports how many runs the index holds."),
		),
		mcp.WithString("session_id",
			mcp.Description("A session id from the list: returns that session's runs of the skill with their cost instead of the list."),
		),
		mcp.WithString("by",
			mcp.Description("\"agent_type\": instead of the list, every agent type and model across the skill's runs of the window — in how many runs, the median count per run and the median token usage of one agent. session_id wins when both are given."),
		),
	)
	server.AddTool(skillRuns, counted(counter, "skill_runs", skillRunsHandler(indexer, store)))
}

func sessionGetHandler(
	differ *watcher.Differ,
	loader *watcher.TurnLoader,
	pageStore *PageStore[*sessionGetResult],
	s *session.Store,
) server.ToolHandlerFunc {
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
		withDiff := boolArgFromRequest(request, "diff", false)
		withUncommitted := boolArgFromRequest(request, "uncommitted_diff", false)
		withMemory := boolArgFromRequest(request, "remember", false)
		n := intArgFromRequest(request, "n")
		if n <= 0 {
			n = DefaultReturnedTurns
		}

		subagentId, _ := args["subagent"].(string)
		withThinking := boolArgFromRequest(request, "thinking", false)

		withTools := boolArgFromRequest(request, "tools", false)

		var bufferedTurns []*session.Turn
		var source *TurnSource
		var header *sessionGetResult
		isKnownSubagent := true
		isFound := s.WithSession(sess.Meta.SessionId, func(lockedSession *session.Session) {
			bufferedTurns, isKnownSubagent = scopeTurns(lockedSession, n, subagentId, withTools)
			source = NewTurnSource(lockedSession, subagentId, withTools)
			turnsTotal := scopeTurnsTotal(lockedSession, subagentId, withTools)
			header = &sessionGetResult{
				LastActive:    lockedSession.LastActive,
				PlanRevisions: newPlanRevisionsView(lockedSession),
				Subagents:     newSubagentRefs(lockedSession),
				TotalUsage:    lockedSession.CurrentUsage(),
				TurnsTotal:    &turnsTotal,
			}
		})
		if !isFound {
			return mcp.NewToolResultError(fmt.Sprintf("session %q not found", sess.Meta.SessionId)), nil
		}

		if !isKnownSubagent {
			return mcp.NewToolResultError(fmt.Sprintf("unknown subagent id %q; valid ids: %v", subagentId, sess.SubagentIds())), nil
		}

		var scopedTurns []*turnView
		if withTurns {
			scopedTurns = newTurnViews(source.Turns(loader, bufferedTurns, n), withThinking, withTools)
		}

		scopedEvents := sess.Events.All()
		if subagentId != "" {
			scopedEvents = filterEventsByActor(scopedEvents, subagentId)
			withPlan, withDiff, withUncommitted, withMemory = false, false, false, false
		}

		var diff, diffTarget, uncommitted string
		if withDiff {
			diff, diffTarget, _ = differ.SessionDiff(ctx, sess.Meta.SessionId)
		}
		if withUncommitted {
			uncommitted, _ = differ.UncommittedDiff(ctx, sess.Meta.SessionId)
		}

		if boolArgFromRequest(request, "json", false) {
			result := header
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
				result.Diff = diff
				result.DiffTarget = diffTarget
			}
			if withUncommitted {
				result.UncommittedDiff = uncommitted
			}
			if withMemory {
				result.Memory = memoryBlock(sess)
			}
			return respondWithStructured(result)
		}

		var turns, events, plan, memory string
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
			firstPage.DiffTarget = diffTarget
		}
		firstPage.LastActive = header.LastActive
		firstPage.PlanRevisions = header.PlanRevisions
		firstPage.Subagents = header.Subagents
		firstPage.TotalUsage = header.TotalUsage
		firstPage.TurnsTotal = header.TurnsTotal

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
				HasDiff:     sess.HasDiff(),
				DiffTarget:  sess.DiffTarget,
				Meta:        sess.Meta,
			}
		}

		return respondWithStructured(map[string]any{"sessions": items})
	}
}

// skillRunsHandler answers from the index alone: a warm instance writes its live sessions first, a cold one is never warmed for it.
func skillRunsHandler(indexer *index.Indexer, s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if indexer == nil {
			return mcp.NewToolResultError(errIndexUnavailable.Error()), nil
		}

		// skill
		skill := strings.TrimSpace(request.GetString("skill", ""))
		if skill == "" {
			return mcp.NewToolResultError(errSkillMissing.Error()), nil
		}

		// limit
		limit := request.GetInt("limit", defaultSkillRuns)
		if limit < 1 {
			return mcp.NewToolResultError(errSkillRunsLimitInvalid.Error()), nil
		}

		if isStoreReady(s) {
			indexer.PersistLive()
		}
		runs := index.RunsOf(indexer.Records(), skill, time.Now())

		// session_id
		sessionId := strings.TrimSpace(request.GetString("session_id", ""))
		if sessionId != "" {
			return respondWithStructured(newSkillRunsDetail(runs, session.Id(sessionId)))
		}

		// by
		switch strings.TrimSpace(request.GetString("by", "")) {
		case "":
		case skillRunsByAgentType:
			return respondWithStructured(newSkillRunsByAgentType(runs))
		default:
			return mcp.NewToolResultError(errSkillRunsByInvalid.Error()), nil
		}

		rows := make([]*skillRunRow, 0, min(limit, len(runs)))
		for _, run := range runs[:min(limit, len(runs))] {
			rows = append(rows, newSkillRunRow(run))
		}
		result := &skillRunsResult{Runs: rows, Total: len(runs)}
		return respondWithStructured(result)
	}
}

func dropSubagentTouchedFiles(views []*SubagentStatView) {
	for _, view := range views {
		view.TouchedFiles = nil
	}
}

func isStoreReady(s *session.Store) bool {
	select {
	case <-s.Ready():
		return true
	default:
		return false
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
		counters.Turns = currentSession.TotalTurns()
		firstPage.Counters = &counters
		firstPage.Diff = diffAvailability(currentSession)
		firstPage.PlanRevisions = newPlanRevisionsView(currentSession)
		firstPage.Time = NewSessionTimeView(currentSession)
		if firstPage.Time != nil {
			firstPage.Time.Telemetry = NewTelemetryTimeView(currentSession, detector, telemetryStore, s.StateDir)
		}
		withDetails := boolArgFromRequest(request, "details", false)
		firstPage.Permissions = newPermissionsView(currentSession, telemetryStore, s.StateDir)
		if firstPage.Permissions != nil && !withDetails {
			firstPage.Permissions.Requests = nil
		}
		if withDetails {
			firstPage.TouchedFiles = newTouchedFileViews(currentSession.TouchedFiles)
		}
		if boolArgFromRequest(request, "breakdown", false) {
			firstPage.Skills = newSkillStatViews(currentSession)
			firstPage.Subagents = NewSubagentStatViews(currentSession)
			if !withDetails {
				dropSubagentTouchedFiles(firstPage.Subagents)
			}
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

// diffAvailability reports live while the working tree exists (computed on request), else snapshot or none.
func diffAvailability(currentSession *session.Session) string {
	cwd := currentSession.Meta.CWD
	if cwd != "" {
		_, err := os.Stat(cwd)
		if err == nil {
			return "live"
		}
	}

	if currentSession.HasDiffSnapshot {
		return "snapshot"
	}
	return "none"
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

func scopeTurns(currentSession *session.Session, number int, subagentId string, withTools bool) ([]*session.Turn, bool) {
	if subagentId != "" {
		if withTools {
			return currentSession.SubagentTurnsWithToolCalls(subagentId, number)
		}
		return currentSession.SubagentTurns(subagentId, number)
	}

	if withTools {
		return currentSession.TurnsWithToolCalls(number), true
	}
	return currentSession.Turns(number), true
}

func scopeTurnsTotal(currentSession *session.Session, subagentId string, withTools bool) int {
	if subagentId != "" {
		if withTools {
			return currentSession.SubagentTotalTurnsWithToolCalls(subagentId)
		}
		return currentSession.SubagentTotalTurns(subagentId)
	}

	if withTools {
		return currentSession.TotalTurnsWithToolCalls()
	}
	return currentSession.TotalTurns()
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
