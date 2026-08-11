package tools

import (
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
		first, next := b.build("d", "e", "m", "p", "t", "u")

		assert.Nil(t, next)
		assert.Equal(t, "t", first.Turns)
		assert.Equal(t, "e", first.Events)
		assert.Equal(t, "m", first.Memory)
		assert.Equal(t, "p", first.Plan)
		assert.Equal(t, "d", first.Diff)
		assert.Equal(t, "u", first.UncommittedDiff)
	})

	// turns-before-events-before-plan
	t.Run("turns-before-events-before-plan", func(t *testing.T) {
		b := NewPageBuilder(5)
		first, next := b.build("", "BBBBB", "", "", "AAAAA", "")

		assert.Equal(t, "AAAAA", first.Turns)
		assert.Empty(t, first.Events)
		require.Len(t, next, 1)
		assert.Equal(t, "BBBBB", next[0].Events)
	})

	// drain-order-all-sections
	t.Run("drain-order-all-sections", func(t *testing.T) {
		b := NewPageBuilder(4)
		first, next := b.build("dddd", "eeee", "mmmm", "pppp", "tttt", "uuuu")

		pages := append([]*sessionGetResult{first}, next...)
		require.Len(t, pages, 6)
		assert.Equal(t, "tttt", pages[0].Turns)
		assert.Equal(t, "eeee", pages[1].Events)
		assert.Equal(t, "pppp", pages[2].Plan)
		assert.Equal(t, "dddd", pages[3].Diff)
		assert.Equal(t, "uuuu", pages[4].UncommittedDiff)
		assert.Equal(t, "mmmm", pages[5].Memory)
	})

	// uncommitted-diff-before-memory
	t.Run("uncommitted-diff-before-memory", func(t *testing.T) {
		b := NewPageBuilder(6)
		first, next := b.build("dddd", "", "mm", "", "", "uu")

		pages := append([]*sessionGetResult{first}, next...)
		require.Len(t, pages, 2)
		assert.Equal(t, "dddd", pages[0].Diff)
		assert.Equal(t, "uu", pages[0].UncommittedDiff)
		assert.Equal(t, "mm", pages[1].Memory)
	})

	// memory-drains-last
	t.Run("memory-drains-last", func(t *testing.T) {
		b := NewPageBuilder(3)
		first, next := b.build("", "", "MMMM", "", "TT", "")

		assert.Equal(t, "TT", first.Turns)
		assert.Equal(t, "M", first.Memory)
		require.Len(t, next, 1)
		assert.Equal(t, "MMM", next[0].Memory)
	})

	// utf8-boundary-respected
	t.Run("utf8-boundary-respected", func(t *testing.T) {
		turns := strings.Repeat("é", 10) // 20 bytes
		b := NewPageBuilder(5)
		first, next := b.build("", "", "", "", turns, "")

		assert.True(t, utf8.ValidString(first.Turns.(string)))
		for _, page := range next {
			assert.True(t, utf8.ValidString(page.Turns.(string)))
		}
	})
}

func TestBuildEvents(t *testing.T) {
	// single-page-complete-strings
	t.Run("single-page-complete-strings", func(t *testing.T) {
		b := NewPageBuilder(1000)
		first, next := b.buildEvents(`[{"kind":"skill_invoked"}]`, `[{"index":1}]`)

		assert.Nil(t, next)
		assert.Equal(t, `[{"kind":"skill_invoked"}]`, first.Events)
		assert.Equal(t, `[{"index":1}]`, first.Revisions)
	})

	// events-before-revisions
	t.Run("events-before-revisions", func(t *testing.T) {
		b := NewPageBuilder(5)
		first, next := b.buildEvents("AAAAA", "BBBBB")

		assert.Equal(t, "AAAAA", first.Events)
		assert.Empty(t, first.Revisions)
		require.Len(t, next, 1)
		assert.Equal(t, "BBBBB", next[0].Revisions)
	})

	// chunks-reassemble-to-original
	t.Run("chunks-reassemble-to-original", func(t *testing.T) {
		events := `[{"kind":"plan_rejected"}]`
		b := NewPageBuilder(10)
		first, next := b.buildEvents(events, "")

		require.Len(t, next, 2)
		reassembled := first.Events.(string)
		for _, page := range next {
			reassembled += page.Events.(string)
		}
		assert.Equal(t, events, reassembled)
	})

	// revisions-fill-remaining-space
	t.Run("revisions-fill-remaining-space", func(t *testing.T) {
		b := NewPageBuilder(3)
		first, next := b.buildEvents("EE", "RRRR")

		assert.Equal(t, "EE", first.Events)
		assert.Equal(t, "R", first.Revisions)
		require.Len(t, next, 1)
		assert.Equal(t, "RRR", next[0].Revisions)
	})

	// utf8-boundary-respected
	t.Run("utf8-boundary-respected", func(t *testing.T) {
		events := strings.Repeat("é", 10) // 20 bytes
		b := NewPageBuilder(5)
		first, next := b.buildEvents(events, "")

		assert.True(t, utf8.ValidString(first.Events.(string)))
		for _, page := range next {
			assert.True(t, utf8.ValidString(page.Events.(string)))
		}
	})
}
