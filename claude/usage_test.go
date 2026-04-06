package claude

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
