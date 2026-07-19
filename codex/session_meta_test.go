package codex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func provideCompleteSessionMeta() *SessionMeta {
	return &SessionMeta{
		Id:         "sess-123",
		CWD:        "/project",
		CLIVersion: "1.0.0",
	}
}

func TestSource_UnmarshalJSON(t *testing.T) {
	type testCase struct {
		_expected Source
		_id       string

		data string
	}

	tests := make([]*testCase, 0)

	// string-source
	test := &testCase{
		_id:       "string-source",
		_expected: Source{Kind: "vscode"},
		data:      `"vscode"`,
	}
	tests = append(tests, test)

	// subagent-object
	test = &testCase{
		_id: "subagent-object",
		_expected: Source{
			AgentNickname:  "Hume",
			Kind:           SourceKindSubagent,
			ParentThreadId: "sess-parent",
		},
		data: `{"subagent":{"thread_spawn":{"parent_thread_id":"sess-parent","agent_nickname":"Hume"}}}`,
	}
	tests = append(tests, test)

	// malformed-object
	test = &testCase{
		_id:       "malformed-object",
		_expected: Source{Kind: SourceKindUnknown},
		data:      `{"something":"else"}`,
	}
	tests = append(tests, test)

	// non-string-non-object
	test = &testCase{
		_id:       "non-string-non-object",
		_expected: Source{Kind: SourceKindUnknown},
		data:      `42`,
	}
	tests = append(tests, test)

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			var source Source
			err := source.UnmarshalJSON([]byte(test.data))
			assert.NoError(t, err)
			assert.Equal(t, test._expected, source)
		})
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
	form.Id = ""
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
