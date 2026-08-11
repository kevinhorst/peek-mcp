package tools

import (
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTouchedFileViews(t *testing.T) {
	// sorted-by-path
	t.Run("sorted-by-path", func(t *testing.T) {
		s := &session.Session{
			TouchedFiles: map[string]*session.FileTouchCounts{
				"/b.go": {Writes: 1},
				"/a.go": {Reads: 2},
			},
		}

		views := newTouchedFileViews(s)
		require.Len(t, views, 2)
		assert.Equal(t, "/a.go", views[0].Path)
		assert.Equal(t, 2, views[0].Reads)
		assert.Equal(t, "/b.go", views[1].Path)
		assert.Equal(t, 1, views[1].Writes)
	})

	// nil-when-empty
	t.Run("nil-when-empty", func(t *testing.T) {
		assert.Nil(t, newTouchedFileViews(&session.Session{}))
	})
}
