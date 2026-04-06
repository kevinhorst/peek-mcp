package codex

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/models"
	"github.com/stretchr/testify/assert"
)

func provideCompleteEntry() *Entry {
	return &Entry{
		Timestamp: time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC),
		Type:      EntryTypeSessionMeta,
		Payload:   json.RawMessage(`{"id":"sess-123"}`),
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

func provideCompleteSessionMeta() *SessionMeta {
	return &SessionMeta{
		ID:         "sess-123",
		CWD:        "/project",
		CLIVersion: "1.0.0",
	}
}

func TestSessionMeta_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *SessionMeta
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteSessionMeta(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-meta",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteSessionMeta()
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

func provideCompleteTurnContext() *TurnContext {
	return &TurnContext{
		TurnID: "turn-123",
		Model:  "gpt-5.4",
		CWD:    "/project",
	}
}

func TestTurnContext_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *TurnContext
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteTurnContext(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-context",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteTurnContext()
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

func provideCompleteResponseItem() *ResponseItem {
	return &ResponseItem{
		Type: "message",
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "output_text", Text: "done"},
		},
	}
}

func TestResponseItem_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *ResponseItem
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteResponseItem(),
	}
	tests = append(tests, test)

	form := provideCompleteResponseItem()
	form.Role = models.RoleDeveloper
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

	form = provideCompleteResponseItem()
	form.Type = ""
	test = &testCase{
		_id:         "fail-empty-type",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteResponseItem()
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

func provideCompleteEventMessage() *EventMessage {
	return &EventMessage{
		Type: EventTypeTokenCount,
		Info: &EventInfo{
			TotalTokenUsage: &TokenUsage{
				InputTokens:           100,
				CachedInputTokens:     60,
				OutputTokens:          20,
				ReasoningOutputTokens: 5,
				TotalTokens:           125,
			},
		},
	}
}

func TestEventMessage_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *EventMessage
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteEventMessage(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-event-message",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteEventMessage()
	form.Type = ""
	test = &testCase{
		_id:         "fail-empty-type",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteEventMessage()
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

func provideCompleteContentBlock() *ContentBlock {
	return &ContentBlock{
		Type: "input_text",
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
