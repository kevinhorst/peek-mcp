package tools

import (
	"context"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func boolArgFromRequest(name string, request mcp.CallToolRequest) bool {
	value, ok := request.GetArguments()[name].(bool)
	if !ok {
		return false
	}

	return value
}

func intArgFromRequest(name string, request mcp.CallToolRequest) int {
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

func clientNameFromRequest(ctx context.Context) string {
	s := server.ClientSessionFromContext(ctx)
	if s == nil {
		return ""
	}

	withInfo, ok := s.(server.SessionWithClientInfo)
	if !ok {
		return ""
	}
	name := strings.ToLower(withInfo.GetClientInfo().Name)
	slog.Info("clientName: Resolved client name:", name, "")

	return name
}

func isClaude(ctx context.Context) bool {
	name := clientNameFromRequest(ctx)
	if name == "" {
		return true
	}

	return strings.Contains(name, "claude")
}
