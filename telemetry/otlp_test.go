package telemetry

import (
	"strconv"
	"strings"
	"testing"
	"time"

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

// logsPayload wraps records in the envelope Claude Code emits (captured from a
// real v2.1.153 session; the event name sits in the record body and repeats
// unprefixed in the event.name attribute).
func logsPayload(records ...string) []byte {
	return []byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[` + strings.Join(records, ",") + `]}]}]}`)
}

func decisionRecord(sessionId, decision, source, tool, toolUseId string) string {
	return `{"timeUnixNano":"1787749325706000000","body":{"stringValue":"claude_code.tool_decision"},"attributes":[` +
		`{"key":"session.id","value":{"stringValue":"` + sessionId + `"}},` +
		`{"key":"event.name","value":{"stringValue":"tool_decision"}},` +
		`{"key":"decision","value":{"stringValue":"` + decision + `"}},` +
		`{"key":"source","value":{"stringValue":"` + source + `"}},` +
		`{"key":"tool_name","value":{"stringValue":"` + tool + `"}},` +
		`{"key":"tool_use_id","value":{"stringValue":"` + toolUseId + `"}}]}`
}

func resultRecord(sessionId, toolUseId, toolInput string) string {
	return `{"timeUnixNano":"1787749325778000000","body":{"stringValue":"claude_code.tool_result"},"attributes":[` +
		`{"key":"session.id","value":{"stringValue":"` + sessionId + `"}},` +
		`{"key":"event.name","value":{"stringValue":"tool_result"}},` +
		`{"key":"tool_use_id","value":{"stringValue":"` + toolUseId + `"}},` +
		`{"key":"tool_input","value":{"stringValue":` + strconv.Quote(toolInput) + `}}]}`
}

func TestStore_IngestLogs(t *testing.T) {
	// config-decision-counted-only
	t.Run("config-decision-counted-only", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestLogs(logsPayload(decisionRecord("s1", "accept", "config", "Bash", "tu1"))))

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Equal(t, 1, stats.Permissions.AutoAllowed)
		assert.Empty(t, stats.Permissions.Requests)
	})

	// prompted-decision-listed
	t.Run("prompted-decision-listed", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestLogs(logsPayload(decisionRecord("s1", "accept", "user_temporary", "Bash", "tu1"))))

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Equal(t, 1, stats.Permissions.PromptedOnce)
		require.Len(t, stats.Permissions.Requests, 1)
		request := stats.Permissions.Requests[0]
		assert.Equal(t, "accept", request.Decision)
		assert.Equal(t, "user_temporary", request.Source)
		assert.Equal(t, "Bash", request.Tool)
		assert.Equal(t, "tu1", request.ToolUseId)
		assert.Equal(t, time.Unix(0, 1787749325706000000).UTC(), request.Timestamp)
	})

	// rejected-decision-counted-and-listed
	t.Run("rejected-decision-counted-and-listed", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestLogs(logsPayload(decisionRecord("s1", "reject", "user_reject", "Edit", "tu2"))))

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Equal(t, 1, stats.Permissions.Rejected)
		require.Len(t, stats.Permissions.Requests, 1)
		assert.Equal(t, "reject", stats.Permissions.Requests[0].Decision)
	})

	// tool-result-enriches-command
	t.Run("tool-result-enriches-command", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestLogs(logsPayload(
			decisionRecord("s1", "accept", "user_temporary", "Bash", "tu1"),
			resultRecord("s1", "tu1", `{"command":"echo capture-probe","description":"probe"}`),
		)))

		stats, ok := s.Get("s1")
		require.True(t, ok)
		require.Len(t, stats.Permissions.Requests, 1)
		assert.Equal(t, "echo capture-probe", stats.Permissions.Requests[0].Command)
	})

	// config-result-not-tracked
	t.Run("config-result-not-tracked", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestLogs(logsPayload(
			decisionRecord("s1", "accept", "config", "Bash", "tu1"),
			resultRecord("s1", "tu1", `{"command":"echo hi"}`),
		)))

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Empty(t, stats.Permissions.Requests)
	})

	// missing-session-id-skipped
	t.Run("missing-session-id-skipped", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestLogs(logsPayload(`{"body":{"stringValue":"claude_code.tool_decision"},"attributes":[{"key":"decision","value":{"stringValue":"accept"}}]}`)))
		_, ok := s.Get("s1")
		assert.False(t, ok)
	})

	// other-event-skipped
	t.Run("other-event-skipped", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestLogs(logsPayload(`{"body":{"stringValue":"claude_code.api_request"},"attributes":[{"key":"session.id","value":{"stringValue":"s1"}}]}`)))
		_, ok := s.Get("s1")
		assert.False(t, ok)
	})

	// event-name-attribute-fallback
	t.Run("event-name-attribute-fallback", func(t *testing.T) {
		s := NewStore()
		require.NoError(t, s.IngestLogs(logsPayload(`{"attributes":[{"key":"session.id","value":{"stringValue":"s1"}},{"key":"event.name","value":{"stringValue":"tool_decision"}},{"key":"decision","value":{"stringValue":"accept"}},{"key":"source","value":{"stringValue":"config"}}]}`)))

		stats, ok := s.Get("s1")
		require.True(t, ok)
		assert.Equal(t, 1, stats.Permissions.AutoAllowed)
	})

	// invalid-json-error
	t.Run("invalid-json-error", func(t *testing.T) {
		s := NewStore()
		assert.Error(t, s.IngestLogs([]byte("{not json")))
	})
}

func TestCommandFromToolInput(t *testing.T) {
	// bash-command
	assert.Equal(t, "echo hi", commandFromToolInput(`{"command":"echo hi","description":"d"}`))

	// file-path-fallback
	assert.Equal(t, "/etc/hosts", commandFromToolInput(`{"file_path":"/etc/hosts","old_string":"a"}`))

	// notebook-path-fallback
	assert.Equal(t, "/n.ipynb", commandFromToolInput(`{"notebook_path":"/n.ipynb"}`))

	// empty-input
	assert.Empty(t, commandFromToolInput(""))

	// invalid-json
	assert.Empty(t, commandFromToolInput("{not json"))

	// no-known-fields
	assert.Empty(t, commandFromToolInput(`{"query":"x"}`))
}
