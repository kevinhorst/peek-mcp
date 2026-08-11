package tools

import (
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSubagentStatViews(t *testing.T) {
	base := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)

	// sorted-by-start-with-seconds
	t.Run("sorted-by-start-with-seconds", func(t *testing.T) {
		s := &session.Session{
			Subagents: map[string]*session.SubagentStat{
				"sub-2": {FirstActive: base.Add(time.Hour), LastActive: base.Add(time.Hour + time.Minute)},
				"sub-1": {AgentType: "Explore", FirstActive: base, LastActive: base.Add(2 * time.Minute), Usage: session.Usage{InputTokens: 9}},
			},
		}

		views := newSubagentStatViews(s)
		require.Len(t, views, 2)
		assert.Equal(t, "sub-1", views[0].AgentId)
		assert.Equal(t, 120, views[0].Seconds)
		assert.Equal(t, 9, views[0].Usage.InputTokens)
		assert.Equal(t, "sub-2", views[1].AgentId)
	})

	// nil-when-empty
	t.Run("nil-when-empty", func(t *testing.T) {
		assert.Nil(t, newSubagentStatViews(&session.Session{}))
	})
}

func TestNewSkillStatViews(t *testing.T) {
	base := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)

	// closed-window-uses-ended-at
	t.Run("closed-window-uses-ended-at", func(t *testing.T) {
		s := &session.Session{
			Skills: []*session.SkillStat{{Skill: "fchange", StartedAt: base, EndedAt: base.Add(time.Minute)}},
		}

		views := newSkillStatViews(s)
		require.Len(t, views, 1)
		assert.Equal(t, 60, views[0].Seconds)
	})

	// open-window-ends-at-last-active
	t.Run("open-window-ends-at-last-active", func(t *testing.T) {
		s := &session.Session{
			LastActive: base.Add(5 * time.Minute),
			Skills:     []*session.SkillStat{{Skill: "fchange", StartedAt: base}},
		}

		views := newSkillStatViews(s)
		require.Len(t, views, 1)
		assert.Equal(t, 300, views[0].Seconds)
		assert.Equal(t, base.Add(5*time.Minute), views[0].EndedAt)
	})
}
