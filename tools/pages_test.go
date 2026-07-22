package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	// single-page-all-segments
	t.Run("single-page-all-segments", func(t *testing.T) {
		b := NewPageBuilder(1000)
		first, next := b.build("d", "e", "m", "p", "t")

		assert.Nil(t, next)
		assert.Equal(t, "t", first.Turns)
		assert.Equal(t, "e", first.Events)
		assert.Equal(t, "m", first.Memory)
		assert.Equal(t, "p", first.Plan)
		assert.Equal(t, "d", first.Diff)
	})

	// turns-before-events-before-plan
	t.Run("turns-before-events-before-plan", func(t *testing.T) {
		b := NewPageBuilder(5)
		first, next := b.build("", "BBBBB", "", "", "AAAAA")

		assert.Equal(t, "AAAAA", first.Turns)
		assert.Empty(t, first.Events)
		require.Len(t, next, 1)
		assert.Equal(t, "BBBBB", next[0].Events)
	})

	// memory-drains-last
	t.Run("memory-drains-last", func(t *testing.T) {
		b := NewPageBuilder(3)
		first, next := b.build("", "", "MMMM", "", "TT")

		assert.Equal(t, "TT", first.Turns)
		assert.Equal(t, "M", first.Memory)
		require.Len(t, next, 1)
		assert.Equal(t, "MMM", next[0].Memory)
	})

	// utf8-boundary-respected
	t.Run("utf8-boundary-respected", func(t *testing.T) {
		turns := strings.Repeat("é", 10) // 20 bytes
		b := NewPageBuilder(5)
		first, next := b.build("", "", "", "", turns)

		assert.True(t, utf8.ValidString(first.Turns))
		for _, page := range next {
			assert.True(t, utf8.ValidString(page.Turns))
		}
	})
}

func TestBuildEvents(t *testing.T) {
	// single-page-complete-segments-raw-json
	t.Run("single-page-complete-segments-raw-json", func(t *testing.T) {
		b := NewPageBuilder(1000)
		first, next := b.buildEvents(`[{"kind":"skill_invoked"}]`, `[{"index":1}]`)

		assert.Nil(t, next)
		assert.Equal(t, `[{"kind":"skill_invoked"}]`, string(first.Events))
		assert.Equal(t, `[{"index":1}]`, string(first.Revisions))
	})

	// events-before-revisions
	t.Run("events-before-revisions", func(t *testing.T) {
		b := NewPageBuilder(5)
		first, next := b.buildEvents("AAAAA", "BBBBB")

		assert.Equal(t, `"AAAAA"`, string(first.Events))
		assert.Empty(t, first.Revisions)
		require.Len(t, next, 1)
		assert.Equal(t, `"BBBBB"`, string(next[0].Revisions))
	})

	// fragments-ride-as-quoted-strings
	t.Run("fragments-ride-as-quoted-strings", func(t *testing.T) {
		b := NewPageBuilder(10)
		first, next := b.buildEvents(`[{"kind":"plan_rejected"}]`, "")

		require.Len(t, next, 2)
		assert.Equal(t, `"[{\"kind\":\""`, string(first.Events))
		assert.False(t, json.Valid([]byte(`[{"kind":"`)))
	})

	// revisions-fill-remaining-space
	t.Run("revisions-fill-remaining-space", func(t *testing.T) {
		b := NewPageBuilder(3)
		first, next := b.buildEvents("EE", "RRRR")

		assert.Equal(t, `"EE"`, string(first.Events))
		assert.Equal(t, `"R"`, string(first.Revisions))
		require.Len(t, next, 1)
		assert.Equal(t, `"RRR"`, string(next[0].Revisions))
	})

	// utf8-boundary-respected
	t.Run("utf8-boundary-respected", func(t *testing.T) {
		events := strings.Repeat("é", 10) // 20 bytes
		b := NewPageBuilder(5)
		first, next := b.buildEvents(events, "")

		assert.True(t, utf8.Valid(first.Events))
		for _, page := range next {
			assert.True(t, utf8.Valid(page.Events))
		}
	})
}
