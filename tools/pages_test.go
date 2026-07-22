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
