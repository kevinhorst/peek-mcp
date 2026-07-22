package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	MaxResponseBytesClaude = 100 * 1024 // 100Kb - Claude Code Desktop ignored all attempts at increasing it
	MaxResponseBytesCodex  = 0          // 0 = no pagination
)

func respond(ctx context.Context, request mcp.CallToolRequest, response any) (*mcp.CallToolResult, error) {
	if boolArgFromRequest("json", request) {
		return respondWithStructured(response)
	}
	if isClaude(ctx) {
		return respondWithText(response)
	}
	return respondWithStructured(response)
}

func respondForRequest(request mcp.CallToolRequest, response any) (*mcp.CallToolResult, error) {
	if boolArgFromRequest("json", request) {
		return respondWithStructured(response)
	}
	return respondWithText(response)
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

func respondWithText(response any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshaling turns: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

func maxResponseBytes(ctx context.Context) int {
	if isClaude(ctx) {
		return MaxResponseBytesClaude
	}
	return MaxResponseBytesCodex
}
