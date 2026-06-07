package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	DefaultReturnedTurns   = 5
	MaxResponseBytesClaude = 800 * 1024 // 800KB — Claude's 1MB limit with headroom
	MaxResponseBytesCodex  = 0          // 0 = no pagination
)

func maxResponseBytes(ctx context.Context) int {
	s := server.ClientSessionFromContext(ctx)
	if s == nil {
		return MaxResponseBytesClaude
	}

	withInfo, ok := s.(server.SessionWithClientInfo)
	if !ok {
		return MaxResponseBytesClaude
	}

	name := strings.ToLower(withInfo.GetClientInfo().Name)
	slog.Debug("resolved MCP client", "name", name)

	if strings.Contains(name, "codex") {
		return MaxResponseBytesCodex
	}
	return MaxResponseBytesClaude
}

func Register(server *server.MCPServer, store *session.Store) {
	pageStore := &PageStore{
		PagesByRequestId: make(map[string]<-chan *sessionFullResult),
	}

	server.AddTool(
		mcp.NewTool("session_full",
			mcp.WithDescription("Returns turns, plan, and git diff for a session in one call. Prefer this over calling session_latest, session_plan, and session_diff separately. Responses are paginated: if has_more is true, call again with the returned request_id to get the next page."),
			mcp.WithString("id",
				mcp.Description("Session ID (omit for most recent session)"),
			),
			mcp.WithNumber("n",
				mcp.Description("Number of turns to return (default 5)"),
			),
			mcp.WithNumber("diff_size",
				mcp.Description("Max bytes for diff output (0 = no limit)"),
			),
			mcp.WithString("agent",
				mcp.Required(),
				mcp.Description("Agent: \"claude\" or \"codex\". Defaults to the only enabled agent when just one is configured."),
			),
			mcp.WithString("request_id",
				mcp.Description("Pagination request ID from a previous response. Pass this to get the next page."),
			),
		),
		sessionFullHandler(store, pageStore),
	)

	server.AddTool(
		mcp.NewTool("session_latest",
			mcp.WithDescription("Returns the last N human/assistant turn pairs from the most recently active session. Tool calls and tool results are filtered out."),
			mcp.WithNumber("n",
				mcp.Description("Number of turns to return (default 5)"),
			),
			mcp.WithString("agent",
				mcp.Required(),
				mcp.Description("Agent: \"claude\" or \"codex\""),
			),
		),
		sessionLatestHandler(store),
	)

	server.AddTool(
		mcp.NewTool("session_list",
			mcp.WithDescription("Lists all sessions. Returns session ID, agent, last activity timestamp, and whether a plan or diff is available."),
			mcp.WithString("agent",
				mcp.Required(),
				mcp.Description("Agent: \"claude\" or \"codex\""),
			),
		),
		sessionListHandler(store),
	)

	server.AddTool(
		mcp.NewTool("session_get",
			mcp.WithDescription("Returns the last N turns from a specific session by ID."),
			mcp.WithString("id",
				mcp.Description("Session ID"),
				mcp.Required(),
			),
			mcp.WithNumber("n",
				mcp.Description("Number of turns to return (default 5)"),
			),
		),
		sessionGetHandler(store),
	)

	server.AddTool(
		mcp.NewTool("session_plan",
			mcp.WithDescription("Returns the current plan for the given session (or the most recently active session if no ID is provided). Returns an empty response if the session has no plan."),
			mcp.WithString("id",
				mcp.Description("Session ID (optional, defaults to the most recently active session)"),
			),
			mcp.WithString("agent",
				mcp.Required(),
				mcp.Description("Agent: \"claude\" or \"codex\""),
			),
		),
		sessionPlanHandler(store),
	)

	server.AddTool(
		mcp.NewTool("session_diff",
			mcp.WithDescription("Returns the pre-computed git diff for a session. The diff is run against the configured target branch (default: main) in the session's working directory, and refreshed automatically on each new turn. If id is omitted, uses the most recent session."),
			mcp.WithString("id",
				mcp.Description("Session ID (omit for most recent session)"),
			),
			mcp.WithNumber("size",
				mcp.Description("Max bytes to return from diff output (0 = no limit)"),
			),
			mcp.WithString("agent",
				mcp.Required(),
				mcp.Description("Agent: \"claude\" or \"codex\""),
			),
		),
		sessionDiffHandler(store),
	)

	server.AddTool(
		mcp.NewTool("session_uncommitted_diff",
			mcp.WithDescription("Returns the live uncommitted git diff (`git diff HEAD`) for a session, refreshed continuously as files are saved. Resolved in the session's own working tree, so it is correct inside linked git worktrees. If id is omitted, uses the most recent session."),
			mcp.WithString("id",
				mcp.Description("Session ID (omit for most recent session)"),
			),
			mcp.WithNumber("size",
				mcp.Description("Max bytes to return from diff output (0 = no limit)"),
			),
			mcp.WithString("agent",
				mcp.Required(),
				mcp.Description("Agent: \"claude\" or \"codex\""),
			),
		),
		sessionUncommittedDiffHandler(store),
	)
}

func sessionFullHandler(s *session.Store, pageStore *PageStore) server.ToolHandlerFunc {
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
			return respondWithStructured(result)
		}

		// First call: resolve session and build pages
		agent, err := resolveAgent(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var sess *session.Session
		if id, ok := args["id"].(string); ok && id != "" {
			found, ok := s.GetById(session.Id(id))
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("session %q not found", id)), nil
			}
			sess = found
		} else {
			found, ok := s.Last(agent)
			if !ok {
				return mcp.NewToolResultText("no sessions found"), nil
			}
			sess = found
		}

		n := intArgFromRequest(request, "n")
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

		firstPage, nextPages := NewPageBuilder(maxResponseBytes(ctx)).build(turns, plan, diff)

		resultPage := newSessionFullResultPage(firstPage)
		if len(nextPages) == 0 {
			return respondWithStructured(resultPage)
		}

		requestId := uuid.NewString()
		pageStore.add(requestId, nextPages)

		resultPage.WithRequestId(requestId)
		return respondWithStructured(resultPage)
	}
}

func sessionLatestHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agent, err := resolveAgent(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		turnNumber := DefaultReturnedTurns
		n := intArgFromRequest(request, "n")
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

		return respondWithText(turns)
	}
}

func sessionListHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agent, err := resolveAgent(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		sessions := s.List(agent)
		items := make([]sessionListItem, len(sessions))
		for i, sess := range sessions {
			items[i] = sessionListItem{
				Id:         sess.Meta.SessionId,
				Agent:      sess.Agent,
				LastActive: sess.LastActive,
				HasPlan:    sess.PlanContent != "" || sess.PlanFilePath != "",
				HasDiff:    sess.DiffOutput != "",
			}
		}

		return respondWithStructured(map[string]any{"sessions": items})
	}
}

func sessionGetHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		id, ok := args["id"].(string)
		if !ok || id == "" {
			return mcp.NewToolResultError("id parameter is required"), nil
		}

		turnNumber := DefaultReturnedTurns
		n := intArgFromRequest(request, "n")
		if n > 0 {
			turnNumber = n
		}

		currentSession, ok := s.GetById(session.Id(id))
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("session %q not found", id)), nil
		}

		turns := currentSession.Turns(turnNumber)
		if len(turns) == 0 {
			return mcp.NewToolResultError("No turns found"), nil
		}

		return respondWithText(turns)
	}
}

func sessionPlanHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agent, err := resolveAgent(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var currentSession *session.Session

		args := request.GetArguments()
		if id, ok := args["id"].(string); ok && id != "" {
			sess, found := s.GetById(session.Id(id))
			if !found {
				return mcp.NewToolResultError(fmt.Sprintf("session %q not found", id)), nil
			}
			currentSession = sess
		} else {
			sess, ok := s.Last(agent)
			if !ok {
				return respondWithText("No sessions found.")
			}
			currentSession = sess
		}

		if currentSession.PlanContent == "" {
			return mcp.NewToolResultText("No plan found for this session"), nil
		}

		return respondWithText(currentSession.PlanContent)
	}
}

func sessionDiffHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agent, err := resolveAgent(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var currentSession *session.Session

		args := request.GetArguments()
		if idVal, ok := args["id"]; ok && idVal != nil {
			id, _ := idVal.(string)
			if id == "" {
				return mcp.NewToolResultError("id must be a non-empty string"), nil
			}
			sess, found := s.GetById(session.Id(id))
			if !found {
				return mcp.NewToolResultError(fmt.Sprintf("session %q not found", id)), nil
			}
			currentSession = sess
		} else {
			sess, ok := s.Last(agent)
			if !ok {
				return respondWithText("No sessions found.")
			}
			currentSession = sess
		}

		return respondWithText(currentSession.DiffOutput)
	}
}

func sessionUncommittedDiffHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agent, err := resolveAgent(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var currentSession *session.Session

		args := request.GetArguments()
		if idVal, ok := args["id"]; ok && idVal != nil {
			id, _ := idVal.(string)
			if id == "" {
				return mcp.NewToolResultError("id must be a non-empty string"), nil
			}
			sess, found := s.GetById(session.Id(id))
			if !found {
				return mcp.NewToolResultError(fmt.Sprintf("session %q not found", id)), nil
			}
			currentSession = sess
		} else {
			sess, ok := s.Last(agent)
			if !ok {
				return respondWithText("No sessions found.")
			}
			currentSession = sess
		}

		return respondWithText(currentSession.UncommittedDiff)
	}
}

func resolveAgent(s *session.Store, request mcp.CallToolRequest) (session.Agent, error) {
	args := request.GetArguments()
	raw, _ := args["agent"].(string)
	return s.ResolveAgent(session.Agent(raw))
}

func intArgFromRequest(request mcp.CallToolRequest, name string) int {
	args := request.GetArguments()
	value, ok := args[name]
	if !ok {
		return 0
	}

	floatVal, ok := value.(float64)
	if !ok {
		return 0
	}

	return int(floatVal)
}

func respondWithText(response any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshaling turns: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

// respondWithStructured returns StructuredContent with a minimal text fallback.
// mcp-go's NewToolResultJSON duplicates the full payload into both Content[0].text
// and StructuredContent, which doubles the response size (e.g. 9MB → 19MB).
func respondWithStructured(response any) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: "See structuredContent for the full response.",
			},
		},
		StructuredContent: response,
	}, nil
}
