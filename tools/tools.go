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
		mcp.NewTool("session_latest",
			mcp.WithDescription("Returns the last N human/assistant turn pairs from the most recently active session. Tool calls and tool results are filtered out."),
			mcp.WithNumber("n",
				mcp.Description("Number of turns to return (default 5)"),
			),
		),
		sessionLatestHandler(store),
	)

	server.AddTool(
		mcp.NewTool("session_list",
			mcp.WithDescription("Lists all known sessions with metadata: session ID, working directory, git branch, last activity timestamp, total token usage, and model."),
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
}

func sessionLatestHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		turnNumber := DefaultReturnedTurns
		n := intArgFromRequest(request, "n")
		if n > 0 {
			turnNumber = n
		}

		lastSession, ok := s.Last()
		if !ok {
			return mcp.NewToolResultText("session_latest: No sessions found"), nil
		}

		turns, ok := lastSession.Turns.Last(turnNumber)
		if !ok {
			return mcp.NewToolResultText("No turns found"), nil
		}

		return respondWithJson(turns)
	}
}

func sessionListHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return respondWithJson(s.List())
	}
}

func sessionGetHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		id, ok := args["id"].(session.Id)
		if !ok || id == "" {
			return mcp.NewToolResultError("id parameter is required"), nil
		}

		turnNumber := DefaultReturnedTurns
		n := intArgFromRequest(request, "n")
		if n > 0 {
			turnNumber = n
		}

		currentSession, ok := s.Get(id)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("session %q not found", id)), nil
		}

		turns, ok := currentSession.Turns.Last(turnNumber)
		if !ok {
			return mcp.NewToolResultError("No turns found"), nil
		}

		return respondWithJson(turns)
	}
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

func respondWithJson(response any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshaling turns: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}
