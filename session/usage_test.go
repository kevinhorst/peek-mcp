package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func provideCompleteUsage() *Usage {
	return &Usage{
		InputTokens:              100,
		CachedInputTokens:        60,
		OutputTokens:             50,
		ReasoningOutputTokens:    10,
		TotalTokens:              160,
		CacheCreationInputTokens: 200,
		CacheReadInputTokens:     300,
	}
}

func TestUsage_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *Usage
	}

	tests := make([]*testCase, 0)

	// pass-all-ok
	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteUsage(),
	}
	tests = append(tests, test)

	// fail-nil-usage
	test = &testCase{
		_id:         "fail-nil-usage",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	// fail-negative-input-tokens
	form := provideCompleteUsage()
	form.InputTokens = -1
	test = &testCase{
		_id:         "fail-negative-input-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-negative-output-tokens
	form = provideCompleteUsage()
	form.OutputTokens = -1
	test = &testCase{
		_id:         "fail-negative-output-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteUsage()
	form.CachedInputTokens = -1
	test = &testCase{
		_id:         "fail-negative-cached-input-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteUsage()
	form.ReasoningOutputTokens = -1
	test = &testCase{
		_id:         "fail-negative-reasoning-output-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteUsage()
	form.TotalTokens = -1
	test = &testCase{
		_id:         "fail-negative-total-tokens",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// pass-zero-tokens
	form = provideCompleteUsage()
	form.InputTokens = 0
	form.OutputTokens = 0
	test = &testCase{
		_id:         "pass-zero-tokens",
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

func TestUsage_Add(t *testing.T) {
	u := provideCompleteUsage()
	other := &Usage{
		InputTokens:              10,
		CachedInputTokens:        5,
		OutputTokens:             5,
		ReasoningOutputTokens:    2,
		TotalTokens:              20,
		CacheCreationInputTokens: 20,
		CacheReadInputTokens:     30,
	}
	u.Add(other)

	assert.Equal(t, 110, u.InputTokens)
	assert.Equal(t, 65, u.CachedInputTokens)
	assert.Equal(t, 55, u.OutputTokens)
	assert.Equal(t, 12, u.ReasoningOutputTokens)
	assert.Equal(t, 180, u.TotalTokens)
	assert.Equal(t, 220, u.CacheCreationInputTokens)
	assert.Equal(t, 330, u.CacheReadInputTokens)
}

func TestUsage_Add_Nil(t *testing.T) {
	u := provideCompleteUsage()
	u.Add(nil)

	assert.Equal(t, 100, u.InputTokens)
}
