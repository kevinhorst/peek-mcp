package claude

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func provideCompleteEntry() *Entry {
	return &Entry{
		Type:      EntryTypeUser,
		SessionID: "sess-123",
	}
}

func TestEntry_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *Entry
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteEntry(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-entry",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteEntry()
	form.Type = ""
	test = &testCase{
		_id:         "fail-empty-type",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteEntry()
	form.SessionID = ""
	test = &testCase{
		_id:         "fail-empty-session-id",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteEntry()
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

func provideCompleteMessage() *Message {
	return &Message{
		Role:    "assistant",
		Content: json.RawMessage(`[]`),
		Usage: &Usage{
			InputTokens:  1,
			OutputTokens: 2,
		},
	}
}

func TestMessage_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *Message
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteMessage(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-message",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteMessage()
	form.Role = "system"
	test = &testCase{
		_id:         "fail-invalid-role",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteMessage()
	form.Usage = &Usage{InputTokens: -1}
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

func provideCompleteUsage() *Usage {
	return &Usage{
		InputTokens:              1,
		OutputTokens:             2,
		CacheCreationInputTokens: 3,
		CacheReadInputTokens:     4,
	}
}

func TestUsage_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *Usage
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteUsage(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-usage",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteUsage()
	form.InputTokens = -1
	test = &testCase{
		_id:         "fail-negative-input-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteUsage()
	form.OutputTokens = -1
	test = &testCase{
		_id:         "fail-negative-output-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteUsage()
	form.CacheCreationInputTokens = -1
	test = &testCase{
		_id:         "fail-negative-cache-creation-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteUsage()
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

func provideCompleteContentBlock() *ContentBlock {
	return &ContentBlock{
		Type: "text",
		Text: "hello",
	}
}

func TestContentBlock_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *ContentBlock
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteContentBlock(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-block",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteContentBlock()
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
