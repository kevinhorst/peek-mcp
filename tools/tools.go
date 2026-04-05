package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kevinhorst/peek-mcp/store"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func Register(srv *server.MCPServer, s *store.Store) {
	srv.AddTool(
		mcp.NewTool("session_latest",
			mcp.WithDescription("Returns the last N human/assistant turn pairs from the most recently active session. Tool calls and tool results are filtered out."),
			mcp.WithNumber("n",
				mcp.Description("Number of turns to return (default 5)"),
			),
		),
		sessionLatestHandler(s),
	)

	srv.AddTool(
		mcp.NewTool("session_list",
			mcp.WithDescription("Lists all known sessions with metadata: session ID, working directory, git branch, last activity timestamp, total token usage, and model."),
		),
		sessionListHandler(s),
	)

	srv.AddTool(
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
		sessionGetHandler(s),
	)
}

func sessionLatestHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		n := getIntParam(request, "n", 5)

		sess, ok := s.MostRecent()
		if !ok {
			return mcp.NewToolResultText("No sessions found"), nil
		}

		turns := sess.Turns.Last(n)
		data, err := json.Marshal(turns)
		if err != nil {
			return nil, fmt.Errorf("marshaling turns: %w", err)
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

func sessionListHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		list := s.List()
		data, err := json.Marshal(list)
		if err != nil {
			return nil, fmt.Errorf("marshaling sessions: %w", err)
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

func sessionGetHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		id, ok := args["id"].(string)
		if !ok || id == "" {
			return mcp.NewToolResultError("id parameter is required"), nil
		}

		n := getIntParam(request, "n", 5)

		sess, ok := s.Get(id)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("session %q not found", id)), nil
		}

		turns := sess.Turns.Last(n)
		data, err := json.Marshal(turns)
		if err != nil {
			return nil, fmt.Errorf("marshaling turns: %w", err)
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

func getIntParam(request mcp.CallToolRequest, name string, defaultVal int) int {
	args := request.GetArguments()
	v, ok := args[name]
	if !ok {
		return defaultVal
	}
	f, ok := v.(float64)
	if !ok {
		return defaultVal
	}
	if f <= 0 {
		return defaultVal
	}
	return int(f)
}
