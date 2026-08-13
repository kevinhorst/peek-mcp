package telemetry

import (
	"strconv"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_Get(t *testing.T) {
	// miss-returns-false
	t.Run("miss-returns-false", func(t *testing.T) {
		s := NewStore()
		_, ok := s.Get("unknown")
		assert.False(t, ok)
	})

	// eviction-removes-oldest
	t.Run("eviction-removes-oldest", func(t *testing.T) {
		s := NewStore()
		tick := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
		s.now = func() time.Time {
			tick = tick.Add(time.Second)
			return tick
		}

		for i := 0; i < maxSessions; i++ {
			s.fold("s"+strconv.Itoa(i), metricActiveTime, 1, true)
		}
		s.fold("s-new", metricActiveTime, 1, true)

		_, oldestGone := s.Get("s0")
		assert.False(t, oldestGone, "oldest session evicted at cap")
		stats, ok := s.Get("s-new")
		require.True(t, ok)
		assert.Equal(t, 1.0, stats.ActiveSeconds)
	})
}

func TestStore_Persist(t *testing.T) {
	// fold-with-state-dir-writes-file
	t.Run("fold-with-state-dir-writes-file", func(t *testing.T) {
		s := NewStore()
		s.StateDir = state.NewDir(t.TempDir())
		s.now = func() time.Time { return time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC) }

		s.fold("s1", metricActiveTime, 42.5, true)
		s.fold("s1", metricCostUsage, 0.25, true)

		stats, ok := ReadPersisted(s.StateDir, "s1")
		require.True(t, ok)
		assert.Equal(t, 42.5, stats.ActiveSeconds)
		assert.Equal(t, 0.25, stats.CostUSD)
		assert.Equal(t, time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC), stats.UpdatedAt)
	})

	// nil-state-dir-writes-nothing
	t.Run("nil-state-dir-writes-nothing", func(t *testing.T) {
		s := NewStore()
		s.fold("s1", metricActiveTime, 1, true)

		_, ok := ReadPersisted(state.NewDir(t.TempDir()), "s1")
		assert.False(t, ok)
	})

	// read-persisted-nil-dir
	t.Run("read-persisted-nil-dir", func(t *testing.T) {
		_, ok := ReadPersisted(nil, "s1")
		assert.False(t, ok)
	})

	// read-persisted-invalid-json
	t.Run("read-persisted-invalid-json", func(t *testing.T) {
		dir := state.NewDir(t.TempDir())
		require.NoError(t, dir.WriteTelemetry("claude", "s1", "not json"))

		_, ok := ReadPersisted(dir, "s1")
		assert.False(t, ok)
	})
}
