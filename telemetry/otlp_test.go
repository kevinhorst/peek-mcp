package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func metricsPayload(name string, temporality int, dataPoints string) []byte {
	return []byte(`{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"` + name + `","sum":{"aggregationTemporality":` + string(rune('0'+temporality)) + `,"dataPoints":[` + dataPoints + `]}}]}]}]}`)
}

const activePoint = `{"attributes":[{"key":"session.id","value":{"stringValue":"s1"}}],"asDouble":42.5}`

func TestStore_IngestMetrics(t *testing.T) {
	// delta-summed
	t.Run("delta-summed", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestMetrics(metricsPayload(metricActiveTime, temporalityDelta, activePoint)))
		require.NoError(t, s.IngestMetrics(metricsPayload(metricActiveTime, temporalityDelta, activePoint)))

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Equal(t, 85.0, stats.ActiveSeconds)
	})

	// cumulative-keep-max
	t.Run("cumulative-keep-max", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestMetrics(metricsPayload(metricActiveTime, 2, activePoint)))
		require.NoError(t, s.IngestMetrics(metricsPayload(metricActiveTime, 2, `{"attributes":[{"key":"session.id","value":{"stringValue":"s1"}}],"asDouble":40}`)))

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Equal(t, 42.5, stats.ActiveSeconds)
	})

	// cost-folded
	t.Run("cost-folded", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestMetrics(metricsPayload(metricCostUsage, temporalityDelta, `{"attributes":[{"key":"session.id","value":{"stringValue":"s1"}}],"asDouble":0.25}`)))

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Equal(t, 0.25, stats.CostUSD)
	})

	// as-int-string-parsed
	t.Run("as-int-string-parsed", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestMetrics(metricsPayload(metricActiveTime, temporalityDelta, `{"attributes":[{"key":"session.id","value":{"stringValue":"s1"}}],"asInt":"7"}`)))

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Equal(t, 7.0, stats.ActiveSeconds)
	})

	// missing-session-id-skipped
	t.Run("missing-session-id-skipped", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestMetrics(metricsPayload(metricActiveTime, temporalityDelta, `{"attributes":[],"asDouble":1}`)))

		_, ok := s.Get("s1")
		assert.False(t, ok)
	})

	// unknown-metric-ignored
	t.Run("unknown-metric-ignored", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestMetrics(metricsPayload("claude_code.commit.count", temporalityDelta, activePoint)))

		_, ok := s.Get("s1")
		assert.False(t, ok)
	})

	// malformed-json-errors
	t.Run("malformed-json-errors", func(t *testing.T) {
		s := NewStore()
		assert.Error(t, s.IngestMetrics([]byte("{not json")))
	})

	// empty-payload-no-state
	t.Run("empty-payload-no-state", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestMetrics([]byte("{}")))
		_, ok := s.Get("s1")
		assert.False(t, ok)
	})
}
