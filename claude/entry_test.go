package claude

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func provideCompleteClaudeEntry() *ClaudeEntry {
	return &ClaudeEntry{
		Type:      ClaudeEntryTypeUser,
		SessionID: "sess-123",
	}
}

func TestClaudeEntry_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *ClaudeEntry
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteClaudeEntry(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-entry",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteClaudeEntry()
	form.Type = ""
	test = &testCase{
		_id:         "fail-empty-type",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteClaudeEntry()
	form.SessionID = ""
	test = &testCase{
		_id:         "fail-empty-session-id",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteClaudeEntry()
	form.Type = "queue-operation"
	form.SessionID = ""
	test = &testCase{
		_id:         "pass-non-conversation-entry",
		_shouldPass: true,
		form:        form,
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			err := test.form.Validate()
			assert.Equalf(t, test._shouldPass, err == nil, "Err: %v", err)
		})
	}
}

func provideCompleteClaudeMessage() *ClaudeMessage {
	return &ClaudeMessage{
		Role:    "assistant",
		Content: json.RawMessage(`[]`),
		Usage: &ClaudeUsage{
			InputTokens:  1,
			OutputTokens: 2,
		},
	}
}

func TestClaudeMessage_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *ClaudeMessage
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteClaudeMessage(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-message",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteClaudeMessage()
	form.Role = "system"
	test = &testCase{
		_id:         "fail-invalid-role",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteClaudeMessage()
	form.Usage = &ClaudeUsage{InputTokens: -1}
	test = &testCase{
		_id:         "fail-invalid-usage",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			err := test.form.Validate()
			assert.Equalf(t, test._shouldPass, err == nil, "Err: %v", err)
		})
	}
}

func provideCompleteClaudeUsage() *ClaudeUsage {
	return &ClaudeUsage{
		InputTokens:              1,
		OutputTokens:             2,
		CacheCreationInputTokens: 3,
		CacheReadInputTokens:     4,
	}
}

func TestClaudeUsage_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *ClaudeUsage
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteClaudeUsage(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-usage",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteClaudeUsage()
	form.InputTokens = -1
	test = &testCase{
		_id:         "fail-negative-input-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteClaudeUsage()
	form.OutputTokens = -1
	test = &testCase{
		_id:         "fail-negative-output-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteClaudeUsage()
	form.CacheCreationInputTokens = -1
	test = &testCase{
		_id:         "fail-negative-cache-creation-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteClaudeUsage()
	form.CacheReadInputTokens = -1
	test = &testCase{
		_id:         "fail-negative-cache-read-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			err := test.form.Validate()
			assert.Equalf(t, test._shouldPass, err == nil, "Err: %v", err)
		})
	}
}

func provideCompleteClaudeContentBlock() *ClaudeContentBlock {
	return &ClaudeContentBlock{
		Type: "text",
		Text: "hello",
	}
}

func TestClaudeContentBlock_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *ClaudeContentBlock
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteClaudeContentBlock(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-block",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteClaudeContentBlock()
	form.Type = ""
	test = &testCase{
		_id:         "fail-empty-type",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			err := test.form.Validate()
			assert.Equalf(t, test._shouldPass, err == nil, "Err: %v", err)
		})
	}
}
