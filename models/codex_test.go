package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func provideCompleteCodexEntry() *CodexEntry {
	return &CodexEntry{
		Timestamp: time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC),
		Type:      CodexEntryTypeSessionMeta,
		Payload:   json.RawMessage(`{"id":"sess-123"}`),
	}
}

func TestCodexEntry_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *CodexEntry
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteCodexEntry(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-entry",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteCodexEntry()
	form.Type = ""
	test = &testCase{
		_id:         "fail-empty-type",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteCodexEntry()
	form.Payload = nil
	test = &testCase{
		_id:         "fail-empty-payload",
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

func provideCompleteCodexSessionMeta() *CodexSessionMeta {
	return &CodexSessionMeta{
		ID:         "sess-123",
		CWD:        "/project",
		CLIVersion: "1.0.0",
	}
}

func TestCodexSessionMeta_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *CodexSessionMeta
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteCodexSessionMeta(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-meta",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteCodexSessionMeta()
	form.ID = ""
	test = &testCase{
		_id:         "fail-empty-id",
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

func provideCompleteCodexTurnContext() *CodexTurnContext {
	return &CodexTurnContext{
		TurnID: "turn-123",
		Model:  "gpt-5.4",
		CWD:    "/project",
	}
}

func TestCodexTurnContext_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *CodexTurnContext
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteCodexTurnContext(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-context",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteCodexTurnContext()
	form.TurnID = ""
	form.Model = ""
	form.CWD = ""
	test = &testCase{
		_id:         "fail-empty-context",
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

func provideCompleteCodexResponseItem() *CodexResponseItem {
	return &CodexResponseItem{
		Type: "message",
		Role: "assistant",
		Content: []CodexContentBlock{
			{Type: "output_text", Text: "done"},
		},
	}
}

func TestCodexResponseItem_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *CodexResponseItem
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteCodexResponseItem(),
	}
	tests = append(tests, test)

	form := provideCompleteCodexResponseItem()
	form.Role = RoleDeveloper
	test = &testCase{
		_id:         "pass-developer-role",
		_shouldPass: true,
		form:        form,
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-item",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form = provideCompleteCodexResponseItem()
	form.Type = ""
	test = &testCase{
		_id:         "fail-empty-type",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteCodexResponseItem()
	form.Role = "system"
	test = &testCase{
		_id:         "fail-invalid-role",
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

func provideCompleteCodexEventMessage() *CodexEventMessage {
	return &CodexEventMessage{
		Type: CodexEventTypeTokenCount,
		Info: &CodexEventInfo{
			TotalTokenUsage: &CodexTokenUsage{
				InputTokens:           100,
				CachedInputTokens:     60,
				OutputTokens:          20,
				ReasoningOutputTokens: 5,
				TotalTokens:           125,
			},
		},
	}
}

func TestCodexEventMessage_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *CodexEventMessage
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteCodexEventMessage(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-event-message",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteCodexEventMessage()
	form.Type = ""
	test = &testCase{
		_id:         "fail-empty-type",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteCodexEventMessage()
	form.Info.TotalTokenUsage.TotalTokens = -1
	test = &testCase{
		_id:         "fail-invalid-token-usage",
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

func provideCompleteCodexContentBlock() *CodexContentBlock {
	return &CodexContentBlock{
		Type: "input_text",
		Text: "hello",
	}
}

func TestCodexContentBlock_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *CodexContentBlock
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteCodexContentBlock(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-block",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteCodexContentBlock()
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
