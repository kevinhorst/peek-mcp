package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	DefaultReturnedTurns = 5
)

func Register(server *server.MCPServer, store *session.Store) {
	server.AddTool(
		mcp.NewTool("session_full",
			mcp.WithDescription("Returns turns, plan, and git diff for a session in one call. Prefer this over calling session_latest, session_plan, and session_diff separately."),
			mcp.WithString("id",
				mcp.Description("Session ID (omit for most recent session)"),
			),
			mcp.WithString("title",
				mcp.Description("Exact session title (matched by normalized hash, case-insensitive)"),
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
		),
		sessionFullHandler(store),
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
			mcp.WithDescription("Returns the last N turns from a specific session by ID or title."),
			mcp.WithString("id",
				mcp.Description("Session ID"),
			),
			mcp.WithString("title",
				mcp.Description("Exact session title (matched by normalized hash, case-insensitive)"),
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
			mcp.WithString("title",
				mcp.Description("Exact session title (matched by normalized hash, case-insensitive)"),
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
			mcp.WithString("title",
				mcp.Description("Exact session title (matched by normalized hash, case-insensitive)"),
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
			mcp.WithString("title",
				mcp.Description("Exact session title (matched by normalized hash, case-insensitive)"),
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

func sessionFullHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agent, err := resolveAgent(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		sess, err := resolveSession(s, request)
		if err != nil {
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

		turns := truncateTurns(sess.Turns(n), DefaultTurnTextMax)
		plan := truncateString(sess.PlanContent, DefaultPlanMax)

		diffMax := DefaultDiffMax
		if size := intArgFromRequest(request, "diff_size"); size > 0 {
			diffMax = size
		}
		diff := truncateString(sess.DiffOutput, diffMax)

		result := &sessionFullResult{
			Turns: turns,
			Plan:  plan,
			Diff:  diff,
		}

		return respondWithStructured(map[string]any{"session": result})
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

		turns := truncateTurns(lastSession.Turns(turnNumber), DefaultTurnTextMax)
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
				Title:      sess.Title,
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
		currentSession, err := resolveSession(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		turnNumber := DefaultReturnedTurns
		n := intArgFromRequest(request, "n")
		if n > 0 {
			turnNumber = n
		}

		turns := truncateTurns(currentSession.Turns(turnNumber), DefaultTurnTextMax)
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

		currentSession, err := resolveSession(s, request)
		if err != nil {
			found, ok := s.Last(agent)
			if !ok {
				return mcp.NewToolResultText("No sessions found"), nil
			}
			currentSession = found
		}

		if currentSession.PlanContent == "" {
			return mcp.NewToolResultText("No plan found for this session"), nil
		}

		return respondWithText(truncateString(currentSession.PlanContent, DefaultPlanMax))
	}
}

func sessionDiffHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agent, err := resolveAgent(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		currentSession, err := resolveSession(s, request)
		if err != nil {
			found, ok := s.Last(agent)
			if !ok {
				return mcp.NewToolResultText("No sessions found"), nil
			}
			currentSession = found
		}

		maxSize := DefaultDiffMax
		if size := intArgFromRequest(request, "size"); size > 0 {
			maxSize = size
		}
		output := truncateString(currentSession.DiffOutput, maxSize)

		return respondWithText(output)
	}
}

func sessionUncommittedDiffHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agent, err := resolveAgent(s, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		currentSession, err := resolveSession(s, request)
		if err != nil {
			found, ok := s.Last(agent)
			if !ok {
				return mcp.NewToolResultText("No sessions found"), nil
			}
			currentSession = found
		}

		maxSize := DefaultUncommDiffMax
		if size := intArgFromRequest(request, "size"); size > 0 {
			maxSize = size
		}
		output := truncateString(currentSession.UncommittedDiff, maxSize)

		return respondWithText(output)
	}
}

// resolveSession looks up a session by id or title from request args.
// Returns an error if neither is provided, or the referenced session is not found.
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
		sess, found := s.GetByTitle(title)
		if !found {
			return nil, fmt.Errorf("no session matching title %q", title)
		}
		return sess, nil
	}

	return nil, fmt.Errorf("id or title parameter is required")
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
