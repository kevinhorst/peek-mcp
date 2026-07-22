package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/session"
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

func Register(server *server.MCPServer, store *session.Store) {
	pageStore := &PageStore[*sessionFullResult]{
		PagesByRequestId: make(map[string]<-chan *sessionFullResult),
	}
	eventsPageStore := &PageStore[*sessionEventsResult]{
		PagesByRequestId: make(map[string]<-chan *sessionEventsResult),
	}

	sessionFull :=
		mcp.NewTool("session_full",
			mcp.WithDescription("Returns turns, plan, and git diff for a session in one call. Prefer this over calling session_latest, session_plan, and session_diff separately. Responses are paginated: if has_more is true, call again with the returned request_id to get the next page."),
			mcp.WithString("id",
				mcp.Description("Session ID (omit for most recent session)"),
			),
			mcp.WithString("title",
				mcp.Description("Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to agent when provided. For Codex, titles come from Codex's session index (thread name)."),
			),
			mcp.WithNumber("n",
				mcp.Description("Number of turns to return (default 20)"),
			),
			mcp.WithString("agent",
				mcp.Description("Agent: \"claude\" or \"codex\". Required when id and title are omitted."),
			),
			mcp.WithString("request_id",
				mcp.Description("Pagination request ID from a previous response. Pass this to get the next page."),
			),
			mcp.WithBoolean("remember",
				mcp.Description("Include the project's auto-memory (MEMORY.md + fact files). Claude sessions only."),
			),
			mcp.WithBoolean("json",
				mcp.Description("Return the response as structuredContent instead of a JSON text block (default false)"),
			),
		)
	sessionFull.Meta = withMaxResultSize()
	server.AddTool(sessionFull, sessionFullHandler(store, pageStore))

	sessionLatest := mcp.NewTool("session_latest",
		mcp.WithDescription("Returns the last N human/assistant turn pairs from the most recently active session. Tool calls and tool results are filtered out."),
		mcp.WithNumber("n",
			mcp.Description("Number of turns to return (default 20)"),
		),
		mcp.WithString("agent",
			mcp.Required(),
			mcp.Description("Agent: \"claude\" or \"codex\""),
		),
		mcp.WithBoolean("json",
			mcp.Description("Return the response as structuredContent instead of a JSON text block (default false)"),
		),
	)
	sessionLatest.Meta = withMaxResultSize()
	server.AddTool(sessionLatest, sessionLatestHandler(store))

	sessionList :=
		mcp.NewTool("session_list",
			mcp.WithDescription("Lists all sessions. Returns session ID, agent, last activity timestamp, whether a plan or diff is available, the inferred diff base branch (diff_target), and session metadata (cwd, git branch, model, origin)."),
			mcp.WithString("agent",
				mcp.Description("Agent: \"claude\" or \"codex\". Lists all sessions when omitted."),
			),
		)
	sessionList.Meta = withMaxResultSize()
	server.AddTool(sessionList, sessionListHandler(store))

	sessionGet := mcp.NewTool("session_get",
		mcp.WithDescription("Returns the last N turns from a specific session by ID or title."),
		mcp.WithString("id",
			mcp.Description("Session ID"),
		),
		mcp.WithString("title",
			mcp.Description("Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to agent when provided. For Codex, titles come from Codex's session index (thread name)."),
		),
		mcp.WithString("agent",
			mcp.Description("Agent: \"claude\" or \"codex\". Scopes title matching when provided."),
		),
		mcp.WithNumber("n",
			mcp.Description("Number of turns to return (default 20)"),
		),
		mcp.WithBoolean("remember",
			mcp.Description("Include the project's auto-memory (MEMORY.md + fact files). Claude sessions only."),
		),
		mcp.WithBoolean("json",
			mcp.Description("Return the response as structuredContent instead of a JSON text block (default false)"),
		),
	)
	sessionGet.Meta = withMaxResultSize()
	server.AddTool(sessionGet, sessionGetHandler(store))

	sessionPlan :=
		mcp.NewTool("session_plan",
			mcp.WithDescription("Returns the current plan for the given session (or the most recently active session if no ID is provided). For Claude sessions this is the plan-mode plan file; for Codex the latest proposed_plan block. Returns an empty response if the session has no plan."),
			mcp.WithString("id",
				mcp.Description("Session ID (optional, defaults to the most recently active session)"),
			),
			mcp.WithString("title",
				mcp.Description("Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to agent when provided. For Codex, titles come from Codex's session index (thread name)."),
			),
			mcp.WithString("agent",
				mcp.Description("Agent: \"claude\" or \"codex\". Required when id and title are omitted."),
			),
			mcp.WithBoolean("json",
				mcp.Description("Return the response as structuredContent instead of a JSON text block (default false)"),
			),
		)
	sessionPlan.Meta = withMaxResultSize()
	server.AddTool(sessionPlan, sessionPlanHandler(store))

	sessionDiff :=
		mcp.NewTool("session_diff",
			mcp.WithDescription("Returns the pre-computed git diff for a session. The base branch is inferred from the session's live checkout (branch creation point from the reflog, falling back to origin/HEAD, then local main/master, then HEAD) and the diff uses merge-base semantics, refreshed automatically on each new turn. The resolved base is exposed as diff_target in session_full and session_list. If id is omitted, uses the most recent session."),
			mcp.WithString("id",
				mcp.Description("Session ID (omit for most recent session)"),
			),
			mcp.WithString("title",
				mcp.Description("Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to agent when provided. For Codex, titles come from Codex's session index (thread name)."),
			),
			mcp.WithString("agent",
				mcp.Description("Agent: \"claude\" or \"codex\". Required when id and title are omitted."),
			),
			mcp.WithBoolean("json",
				mcp.Description("Return the response as structuredContent instead of a JSON text block (default false)"),
			),
		)
	sessionDiff.Meta = withMaxResultSize()
	server.AddTool(sessionDiff, sessionDiffHandler(store))

	sessionUncommittedDiff :=
		mcp.NewTool("session_uncommitted_diff",
			mcp.WithDescription("Returns the live uncommitted git diff (`git diff HEAD`) for a session, refreshed continuously as files are saved. Resolved in the session's own working tree, so it is correct inside linked git worktrees. If id is omitted, uses the most recent session."),
			mcp.WithString("id",
				mcp.Description("Session ID (omit for most recent session)"),
			),
			mcp.WithString("title",
				mcp.Description("Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to agent when provided. For Codex, titles come from Codex's session index (thread name)."),
			),
			mcp.WithString("agent",
				mcp.Description("Agent: \"claude\" or \"codex\". Required when id and title are omitted."),
			),
			mcp.WithBoolean("json",
				mcp.Description("Return the response as structuredContent instead of a JSON text block (default false)"),
			),
		)
	sessionUncommittedDiff.Meta = withMaxResultSize()
	server.AddTool(sessionUncommittedDiff, sessionUncommittedDiffHandler(store))

	sessionEvents := mcp.NewTool("session_events",
		mcp.WithDescription("Returns the typed event stream of a session (plan lifecycle, permission denials, skill invocations, subagent spawns/results, user answers) plus derived counters, token usage totals, plan revision history, and diff availability (live | snapshot | none). Turns are not included — use session_full for those."),
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
		mcp.WithString("request_id",
			mcp.Description("Pagination request ID from a previous response. Pass this to get the next page."),
		),
		mcp.WithBoolean("json",
			mcp.Description("Return the response as structuredContent instead of a JSON text block (default false)"),
		),
	)
	sessionEvents.Meta = withMaxResultSize()
	server.AddTool(sessionEvents, sessionEventsHandler(store, eventsPageStore))
}

func sessionFullHandler(s *session.Store, pageStore *PageStore[*sessionFullResult]) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		// Continuation: return next page for an existing request
		if reqId, ok := args["request_id"].(string); ok && reqId != "" {
			next, ok := pageStore.next(reqId)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("request_id %q not found or expired", reqId)), nil
			}

			if !pageStore.hasNext(reqId) {
				pageStore.remove(reqId)
				reqId = ""
			}

			result := &sessionFullResultPage{
				sessionFullResult: next,
				RequestId:         reqId,
				HasMore:           pageStore.hasNext(reqId),
			}
			return respond(ctx, request, result)
		}

		// First call: resolve session and build pages
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

		n := intArgFromRequest("n", request)
		if n <= 0 {
			n = DefaultReturnedTurns
		}

		data, err := json.Marshal(sess.Turns(n))
		if err != nil {
			return nil, fmt.Errorf("marshaling turns: %w", err)
		}
		turns := string(data)

		diff := sess.DiffOutput
		plan := sess.PlanContent

		events := marshalEventEntries(sess)
		memory := ""
		if boolArgFromRequest("remember", request) {
			memory = marshalMemoryBlock(sess)
		}

		firstPage, nextPages := NewPageBuilder(maxResponseBytes(ctx)).build(
			diff,
			events,
			memory,
			plan,
			turns,
		)
		firstPage.DiffTarget = sess.DiffTarget

		resultPage := newSessionFullResultPage(firstPage)
		if len(nextPages) == 0 {
			return respond(ctx, request, resultPage)
		}

		requestId := uuid.NewString()
		pageStore.add(requestId, nextPages)

		resultPage.WithRequestId(requestId)
		return respond(ctx, request, resultPage)
	}
}

func sessionLatestHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agent, err := resolveAgentFromRequest(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		turnNumber := DefaultReturnedTurns
		n := intArgFromRequest("n", request)
		if n > 0 {
			turnNumber = n
		}

		lastSession, ok := s.Last(agent)
		if !ok {
			return mcp.NewToolResultText("session_latest: No sessions found"), nil
		}

		turns := lastSession.Turns(turnNumber)
		if len(turns) == 0 {
			return mcp.NewToolResultText("No turns found"), nil
		}

		result := &sessionLatestResult{
			Events: newEventEntries(lastSession.Events.All()),
			Turns:  turns,
		}
		return respondForRequest(request, result)
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

func sessionGetHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		currentSession, err := resolveSession(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		turnNumber := DefaultReturnedTurns
		n := intArgFromRequest("n", request)
		if n > 0 {
			turnNumber = n
		}

		turns := currentSession.Turns(turnNumber)
		if len(turns) == 0 {
			return mcp.NewToolResultError("No turns found"), nil
		}

		result := &sessionGetResult{
			Events:     newEventEntries(currentSession.Events.All()),
			TotalUsage: currentSession.CurrentUsage(),
			Turns:      turns,
		}
		if boolArgFromRequest("remember", request) {
			result.Memory = memoryBlock(currentSession)
		}

		return respondForRequest(request, result)
	}
}

func sessionPlanHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		if currentSession.PlanContent == "" {
			return mcp.NewToolResultText("No plan found for this session"), nil
		}

		if boolArgFromRequest("json", request) {
			result := &sessionPlanResult{Plan: currentSession.PlanContent}
			return respondWithStructured(result)
		}
		return respondWithText(currentSession.PlanContent)
	}
}

func sessionDiffHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		result := &sessionDiffResult{
			Diff:       currentSession.DiffOutput,
			DiffTarget: currentSession.DiffTarget,
			Source:     diffAvailability(currentSession),
		}
		if currentSession.DiffSource == session.DiffSourceSnapshot {
			result.CapturedAt = currentSession.DiffCapturedAt.Format(time.RFC3339)
		}
		return respondForRequest(request, result)
	}
}

func sessionUncommittedDiffHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		if boolArgFromRequest("json", request) {
			result := &sessionUncommittedDiffResult{Diff: currentSession.UncommittedDiff}
			return respondWithStructured(result)
		}
		return respondWithText(currentSession.UncommittedDiff)
	}
}

func sessionEventsHandler(s *session.Store, pageStore *PageStore[*sessionEventsResult]) server.ToolHandlerFunc {
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
		if boolArgFromRequest("revisions", request) {
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
		return []string{"skills", "memory", "user_answers", "plan_approval", "subagent_results"}
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
