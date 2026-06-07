package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	MaxResponseBytesClaude = 100 * 1024 // 400KB — stays under maxResultSizeChars ceiling
	MaxResponseBytesCodex  = 0          // 0 = no pagination
)

func respond(ctx context.Context, response any) (*mcp.CallToolResult, error) {
	if isClaude(ctx) {
		return respondWithText(response)
	}
	return respondWithStructured(response)
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
