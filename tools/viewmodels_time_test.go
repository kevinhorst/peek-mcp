package tools

import (
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
)

func TestNewSessionTimeView(t *testing.T) {
	base := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)

	// wall-idle-active-math
	t.Run("wall-idle-active-math", func(t *testing.T) {
		s := &session.Session{
			StartedAt:  base,
			LastActive: base.Add(30 * time.Minute),
			Idle:       10 * time.Minute,
		}

		view := newSessionTimeView(s)
		assert.Equal(t, 1800, view.WallSeconds)
		assert.Equal(t, 600, view.IdleSeconds)
		assert.Equal(t, 1200, view.ActiveSeconds)
		assert.Equal(t, base, view.StartedAt)
	})

	// nil-without-started-at
	t.Run("nil-without-started-at", func(t *testing.T) {
		s := &session.Session{LastActive: base}
		assert.Nil(t, newSessionTimeView(s))
	})

	// single-entry-zero-wall
	t.Run("single-entry-zero-wall", func(t *testing.T) {
		s := &session.Session{StartedAt: base, LastActive: base}
		view := newSessionTimeView(s)
		assert.Equal(t, 0, view.WallSeconds)
		assert.Equal(t, 0, view.IdleSeconds)
		assert.Equal(t, 0, view.ActiveSeconds)
	})
}
