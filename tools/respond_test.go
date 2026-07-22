package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func provideRequest(arguments map[string]any) mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Arguments = arguments
	return request
}

func TestRespond_JsonFlag(t *testing.T) {
	type testCase struct {
		_id                string
		_shouldBeStructure bool
		request            mcp.CallToolRequest
	}

	tests := make([]*testCase, 0)

	// json-true-structured
	tests = append(tests, &testCase{
		_id:                "json-true-structured",
		_shouldBeStructure: true,
		request:            provideRequest(map[string]any{"json": true}),
	})

	// json-false-text
	tests = append(tests, &testCase{
		_id:                "json-false-text",
		_shouldBeStructure: false,
		request:            provideRequest(map[string]any{"json": false}),
	})

	// json-absent-text
	tests = append(tests, &testCase{
		_id:                "json-absent-text",
		_shouldBeStructure: false,
		request:            provideRequest(nil),
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			response := &sessionPlanResult{Plan: "content"}

			result, err := respond(context.Background(), test.request, response)
			assert.NoError(t, err)
			assert.Equal(t, test._shouldBeStructure, result.StructuredContent != nil)

			result, err = respondForRequest(test.request, response)
			assert.NoError(t, err)
			assert.Equal(t, test._shouldBeStructure, result.StructuredContent != nil)
		})
	}
}
