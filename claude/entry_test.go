package claude

import (
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
