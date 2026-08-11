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

func Register(server *server.MCPServer, store *session.Store, telemetryStore *telemetry.Store) {
	pageStore := &PageStore[*sessionGetResult]{
		PagesByRequestId: make(map[string]<-chan *sessionGetResult),
	}
	eventsPageStore := &PageStore[*sessionEventsResult]{
		PagesByRequestId: make(map[string]<-chan *sessionEventsResult),
	}

	sessionGet := mcp.NewTool("session_get",
		mcp.WithDescription("Returns session data (turns, events, plan, git diff, uncommitted diff, auto-memory) for a session. Defaults to the most recently active session when id and title are omitted. Select sections with the turns/events/plan/diff/uncommitted_diff/remember flags. Responses are paginated: if has_more is true, call again with the returned request_id to get the next page."),
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
		mcp.WithString("request_id",
			mcp.Description("Pagination request ID from a previous response. Pass this to get the next page."),
		),
		mcp.WithBoolean("json",
			mcp.Description("Return the response as structuredContent instead of a JSON text block (default false)"),
		),
	)
	sessionGet.Meta = withMaxResultSize()
	server.AddTool(sessionGet, sessionGetHandler(store, pageStore))

	sessionList :=
		mcp.NewTool("session_list",
			mcp.WithDescription("Lists all sessions. Returns session ID, agent, last activity timestamp, whether a plan or diff is available, the inferred diff base branch (diff_target), and session metadata (cwd, git branch, model, origin)."),
			mcp.WithString("agent",
				mcp.Description("Agent: \"claude\" or \"codex\". Lists all sessions when omitted."),
			),
		)
	sessionList.Meta = withMaxResultSize()
	server.AddTool(sessionList, sessionListHandler(store))

	sessionEvents := mcp.NewTool("session_events",
		mcp.WithDescription("Returns the typed event stream of a session (plan lifecycle, permission denials, skill invocations, subagent spawns/results, user answers) plus derived counters, token usage totals, session time (wall/idle/active seconds), touched files, plan revision history, and diff availability (live | snapshot | none). Turns are not included — use session_get for those."),
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
		mcp.WithString("request_id",
			mcp.Description("Pagination request ID from a previous response. Pass this to get the next page."),
		),
		mcp.WithBoolean("json",
			mcp.Description("Return the response as structuredContent instead of a JSON text block (default false)"),
		),
	)
	sessionEvents.Meta = withMaxResultSize()
	server.AddTool(sessionEvents, sessionEventsHandler(store, eventsPageStore, telemetryStore))
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
			return respond(ctx, request, result)
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

		var turns, events, plan, diff, uncommitted, memory string
		if boolArgFromRequest(request, "turns", true) {
			n := intArgFromRequest(request, "n")
			if n <= 0 {
				n = DefaultReturnedTurns
			}
			data, err := json.Marshal(sess.Turns(n))
			if err != nil {
				return nil, fmt.Errorf("marshaling turns: %w", err)
			}
			turns = string(data)
		}
		withDiff := boolArgFromRequest(request, "diff", true)
		if boolArgFromRequest(request, "events", true) {
			events = marshalEventEntries(sess)
		}
		if boolArgFromRequest(request, "plan", true) {
			plan = sess.PlanContent
		}
		if withDiff {
			diff = sess.DiffOutput
		}
		if boolArgFromRequest(request, "uncommitted_diff", false) {
			uncommitted = sess.UncommittedDiff
		}
		if boolArgFromRequest(request, "remember", false) {
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

		resultPage := newSessionGetResultPage(firstPage)
		if len(nextPages) == 0 {
			return respond(ctx, request, resultPage)
		}

		requestId := uuid.NewString()
		pageStore.add(requestId, nextPages)

		resultPage.WithRequestId(requestId)
		return respond(ctx, request, resultPage)
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
		items := make([]sessionListItem, len(sessions))
		for i, sess := range sessions {
			items[i] = sessionListItem{
				Id:          sess.Meta.SessionId,
				Agent:       sess.Agent,
				Title:       sess.Title,
				TitleSource: sess.TitleSource,
				LastActive:  sess.LastActive,
				HasPlan:     sess.PlanContent != "" || sess.PlanFilePath != "",
				HasDiff:     sess.DiffOutput != "",
				DiffTarget:  sess.DiffTarget,
				Meta:        sess.Meta,
			}
		}

		return respondWithStructured(map[string]any{"sessions": items})
	}
}

func sessionEventsHandler(s *session.Store, pageStore *PageStore[*sessionEventsResult], telemetryStore *telemetry.Store) server.ToolHandlerFunc {
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
			return respond(ctx, request, result)
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

		events := marshalEvents(currentSession)
		revisions := ""
		if boolArgFromRequest(request, "revisions", false) {
			revisions = marshalPlanRevisions(currentSession)
		}

		firstPage, nextPages := NewPageBuilder(maxResponseBytes(ctx)).buildEvents(
			events,
			revisions,
		)
		counters := currentSession.Counters
		firstPage.Counters = &counters
		firstPage.Diff = diffAvailability(currentSession)
		firstPage.PlanRevisions = newPlanRevisionsView(currentSession)
		firstPage.Time = newSessionTimeView(currentSession)
		if telemetryStore != nil && firstPage.Time != nil {
			if stats, ok := telemetryStore.Get(string(currentSession.Meta.SessionId)); ok {
				firstPage.Time.Telemetry = &telemetryTimeView{
					ActiveSeconds: int(stats.ActiveSeconds),
					CostUSD:       stats.CostUSD,
				}
			}
		}
		firstPage.TouchedFiles = newTouchedFileViews(currentSession)
		if boolArgFromRequest(request, "breakdown", false) {
			firstPage.Skills = newSkillStatViews(currentSession)
			firstPage.Subagents = newSubagentStatViews(currentSession)
		}
		firstPage.Unsupported = unsupportedSignals(currentSession.Agent)
		firstPage.Usage = currentSession.CurrentUsage()

		resultPage := newSessionEventsResultPage(firstPage)
		if len(nextPages) == 0 {
			return respond(ctx, request, resultPage)
		}

		requestId := uuid.NewString()
		pageStore.add(requestId, nextPages)

		resultPage.WithRequestId(requestId)
		return respond(ctx, request, resultPage)
	}
}

func diffAvailability(currentSession *session.Session) string {
	if currentSession.DiffOutput == "" {
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
		return []string{"skills", "memory", "user_answers", "plan_approval", "subagent_results", "touched_files", "skill_usage", "subagent_usage", "telemetry"}
	}
	return nil
}

func marshalEvents(currentSession *session.Session) string {
	events := currentSession.Events.All()
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

func marshalEventEntries(currentSession *session.Session) string {
	entries := newEventEntries(currentSession.Events.All())
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
