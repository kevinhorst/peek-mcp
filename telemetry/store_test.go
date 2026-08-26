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

func TestStore_FoldDecision(t *testing.T) {
	// requests-capped-counters-keep-counting
	t.Run("requests-capped-counters-keep-counting", func(t *testing.T) {
		s := NewStore()
		for i := 0; i < maxPermissionRequests+10; i++ {
			s.foldDecision("s1", PermissionDecision{Decision: "reject", Source: sourceUserReject, Tool: "Bash"})
		}

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Equal(t, maxPermissionRequests+10, stats.Permissions.Rejected)
		assert.Len(t, stats.Permissions.Requests, maxPermissionRequests)
	})

	// persist-round-trip
	t.Run("persist-round-trip", func(t *testing.T) {
		s := NewStore()
		s.StateDir = state.NewDir(t.TempDir())
		s.foldDecision("s1", PermissionDecision{Decision: "accept", Source: sourceUserPermanent, Tool: "Bash", ToolUseId: "tu1"})
		s.enrichCommand("s1", "tu1", "make build")

		stats, ok := ReadPersisted(s.StateDir, "s1")
		require.True(t, ok)
		assert.Equal(t, 1, stats.Permissions.PromptedAlways)
		require.Len(t, stats.Permissions.Requests, 1)
		assert.Equal(t, "make build", stats.Permissions.Requests[0].Command)
	})

	// old-persisted-file-without-permissions-parses
	t.Run("old-persisted-file-without-permissions-parses", func(t *testing.T) {
		dir := state.NewDir(t.TempDir())
		require.NoError(t, dir.WriteTelemetry("claude", "s1", `{"active_seconds":42.5,"cost_usd":0.25,"updated_at":"2026-08-13T09:00:00Z"}`))

		stats, ok := ReadPersisted(dir, "s1")
		require.True(t, ok)
		assert.Equal(t, 42.5, stats.ActiveSeconds)
		assert.True(t, stats.Permissions.IsZero())
	})

	// enrich-unknown-tool-use-ignored
	t.Run("enrich-unknown-tool-use-ignored", func(t *testing.T) {
		s := NewStore()
		s.foldDecision("s1", PermissionDecision{Decision: "accept", Source: sourceUserTemporary, Tool: "Bash", ToolUseId: "tu1"})
		s.enrichCommand("s1", "never-seen", "rm -rf /")

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Empty(t, stats.Permissions.Requests[0].Command)
	})

	// enrich-session-mismatch-ignored
	t.Run("enrich-session-mismatch-ignored", func(t *testing.T) {
		s := NewStore()
		s.foldDecision("s1", PermissionDecision{Decision: "accept", Source: sourceUserTemporary, Tool: "Bash", ToolUseId: "tu1"})
		s.enrichCommand("other", "tu1", "rm -rf /")

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Empty(t, stats.Permissions.Requests[0].Command)
	})
}
