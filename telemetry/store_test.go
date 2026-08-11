package telemetry

import (
	"strconv"
	"testing"
	"time"

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
