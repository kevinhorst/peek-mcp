package codex

import (
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
)

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
	form.Role = session.RoleDeveloper
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
