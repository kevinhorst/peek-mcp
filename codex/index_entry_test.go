package codex

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func provideCompleteIndexEntry() *IndexEntry {
	return &IndexEntry{
		Id:         "019d3b51-6d67-74d0-82b5-73e126c120e0",
		ThreadName: "Set up pgroll migrations",
		UpdatedAt:  time.Date(2026, 3, 29, 20, 38, 12, 0, time.UTC),
	}
}

func TestIndexEntry_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *IndexEntry
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteIndexEntry(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-entry",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteIndexEntry()
	form.Id = ""
	test = &testCase{
		_id:         "fail-missing-id",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteIndexEntry()
	form.ThreadName = ""
	test = &testCase{
		_id:         "fail-missing-thread-name",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteIndexEntry()
	form.UpdatedAt = time.Time{}
	test = &testCase{
		_id:         "pass-zero-updated-at",
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
