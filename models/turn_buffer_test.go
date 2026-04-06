package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func makeTurn(text string) *Turn {
	return &Turn{
		Role:      "user",
		Text:      text,
		Timestamp: time.Now(),
	}
}

func turnTexts(turns []*Turn) []string {
	out := make([]string, len(turns))
	for i, t := range turns {
		out[i] = t.Text
	}
	return out
}

func TestTurnBuffer_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *TurnBuffer
	}

	tests := make([]*testCase, 0)

	// pass-valid-buffer
	test := &testCase{
		_id:         "pass-valid-buffer",
		_shouldPass: true,
		form:        NewTurnBuffer(10),
	}
	tests = append(tests, test)

	// fail-nil-buffer
	test = &testCase{
		_id:         "fail-nil-buffer",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			err := test.form.Validate()
			assert.Equalf(t, test._shouldPass, err == nil, "Err: %v", err)
		})
	}
}

func TestTurnBuffer_New_ZeroCapacity(t *testing.T) {
	tb := NewTurnBuffer(0)
	assert.Equal(t, 1, tb.max)
}

func TestTurnBuffer_Push_UnderCapacity(t *testing.T) {
	tb := NewTurnBuffer(5)
	tb.Push(makeTurn("a"))
	tb.Push(makeTurn("b"))
	tb.Push(makeTurn("c"))

	assert.Equal(t, 3, tb.Len())
	assert.Equal(t, []string{"a", "b", "c"}, turnTexts(tb.Last(3)))
}

func TestTurnBuffer_Push_AtCapacity(t *testing.T) {
	tb := NewTurnBuffer(3)
	tb.Push(makeTurn("a"))
	tb.Push(makeTurn("b"))
	tb.Push(makeTurn("c"))

	assert.Equal(t, 3, tb.Len())
	assert.Equal(t, []string{"a", "b", "c"}, turnTexts(tb.Last(3)))
}

func TestTurnBuffer_Push_OverCapacity(t *testing.T) {
	tb := NewTurnBuffer(3)
	for _, s := range []string{"a", "b", "c", "d", "e", "f"} {
		tb.Push(makeTurn(s))
	}

	assert.Equal(t, 3, tb.Len())
	assert.Equal(t, []string{"d", "e", "f"}, turnTexts(tb.Last(3)))
}

func TestTurnBuffer_Last_MoreThanCount(t *testing.T) {
	tb := NewTurnBuffer(10)
	tb.Push(makeTurn("a"))
	tb.Push(makeTurn("b"))

	assert.Equal(t, []string{"a", "b"}, turnTexts(tb.Last(5)))
}

func TestTurnBuffer_Last_Zero(t *testing.T) {
	tb := NewTurnBuffer(5)
	tb.Push(makeTurn("a"))

	assert.Nil(t, tb.Last(0))
}

func TestTurnBuffer_Last_EmptyBuffer(t *testing.T) {
	tb := NewTurnBuffer(5)
	assert.Nil(t, tb.Last(3))
}

func TestTurnBuffer_Last_One(t *testing.T) {
	tb := NewTurnBuffer(3)
	tb.Push(makeTurn("a"))
	tb.Push(makeTurn("b"))
	tb.Push(makeTurn("c"))

	assert.Equal(t, []string{"c"}, turnTexts(tb.Last(1)))
}

func TestTurnBuffer_WrapAround_Order(t *testing.T) {
	tb := NewTurnBuffer(3)
	for _, s := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		tb.Push(makeTurn(s))
	}

	assert.Equal(t, []string{"i", "j"}, turnTexts(tb.Last(2)))
}
