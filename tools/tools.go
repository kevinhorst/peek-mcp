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
			mcp.WithNumber("n",
				mcp.Description("Number of turns to return (default 5)"),
			),
			mcp.WithNumber("diff_size",
				mcp.Description("Max bytes for diff output (0 = no limit)"),
			),
			mcp.WithString("agent",
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
				mcp.Description("Agent: \"claude\" or \"codex\". Defaults to the only enabled agent when just one is configured."),
			),
		),
		sessionLatestHandler(store),
	)

	server.AddTool(
		mcp.NewTool("session_list",
			mcp.WithDescription("Lists all sessions. Returns session ID, agent, last activity timestamp, and whether a plan or diff is available."),
			mcp.WithString("agent",
				mcp.Description("Agent: \"claude\" or \"codex\". Defaults to the only enabled agent when just one is configured."),
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
				mcp.Description("Agent: \"claude\" or \"codex\". Defaults to the only enabled agent when just one is configured."),
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
				mcp.Description("Agent: \"claude\" or \"codex\". Defaults to the only enabled agent when just one is configured."),
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
				mcp.Description("Agent: \"claude\" or \"codex\". Defaults to the only enabled agent when just one is configured."),
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

		var sess *session.Session

		args := request.GetArguments()
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

		diff := sess.DiffOutput
		if size := intArgFromRequest(request, "diff_size"); size > 0 && len(diff) > size {
			diff = diff[:size] + fmt.Sprintf("\n[truncated: exceeded %d bytes]", size)
		}

		result := sessionFullResult{
			Turns: sess.Turns(n),
			Plan:  sess.PlanContent,
			Diff:  diff,
		}
		return respondWithJson(map[string]any{"session": result})
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
				HasPlan:    sess.PlanContent != "",
				HasDiff:    sess.DiffOutput != "",
			}
		}

		return respondWithJson(map[string]any{"sessions": items})
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

		output := currentSession.DiffOutput
		size := intArgFromRequest(request, "size")
		if size > 0 && len(output) > size {
			output = output[:size] + fmt.Sprintf("\n[truncated: diff exceeded %d bytes]", size)
		}

		return respondWithText(output)
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

		output := currentSession.UncommittedDiff
		size := intArgFromRequest(request, "size")
		if size > 0 && len(output) > size {
			output = output[:size] + fmt.Sprintf("\n[truncated: diff exceeded %d bytes]", size)
		}

		return respondWithText(output)
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

func respondWithJson(response any) (*mcp.CallToolResult, error) {
	resp, err := mcp.NewToolResultJSON(response)
	if err != nil {
		return nil, fmt.Errorf("creating tool result: %w", err)
	}

	return resp, nil
}
