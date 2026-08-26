package tools

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/kevinhorst/peek-mcp/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestNewTelemetryTimeView(t *testing.T) {
	claudeSession := func() *session.Session {
		return &session.Session{Agent: session.AgentClaude, Meta: session.Meta{SessionId: "s1"}}
	}

	writeSettings := func(t *testing.T, content string) string {
		path := filepath.Join(t.TempDir(), "settings.json")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}

	configuredSettings := `{"env": {
		"CLAUDE_CODE_ENABLE_TELEMETRY": "1",
		"OTEL_METRICS_EXPORTER": "otlp",
		"OTEL_LOGS_EXPORTER": "otlp",
		"OTEL_EXPORTER_OTLP_PROTOCOL": "http/json",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:42442/otlp"
	}}`

	// receiving-overrides-configured
	t.Run("receiving-overrides-configured", func(t *testing.T) {
		store := telemetry.NewStore()
		payload := `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"claude_code.active_time.total","sum":{"aggregationTemporality":1,"dataPoints":[{"attributes":[{"key":"session.id","value":{"stringValue":"s1"}}],"asDouble":42.5}]}}]}]}]}`
		require.NoError(t, store.IngestMetrics([]byte(payload)))
		detector := telemetry.NewDetector(42442, writeSettings(t, configuredSettings))

		view := newTelemetryTimeView(claudeSession(), detector, store, nil)

		assert.Equal(t, telemetry.ExportReceiving, view.Status)
		assert.Equal(t, 42, view.ActiveSeconds)
	})

	// configured-without-data
	t.Run("configured-without-data", func(t *testing.T) {
		detector := telemetry.NewDetector(42442, writeSettings(t, configuredSettings))

		view := newTelemetryTimeView(claudeSession(), detector, telemetry.NewStore(), nil)

		assert.Equal(t, telemetry.ExportConfigured, view.Status)
		assert.Empty(t, view.Detail)
		assert.Equal(t, 0, view.ActiveSeconds)
	})

	// misconfigured-detail-passthrough
	t.Run("misconfigured-detail-passthrough", func(t *testing.T) {
		grpcSettings := `{"env": {
			"CLAUDE_CODE_ENABLE_TELEMETRY": "1",
			"OTEL_METRICS_EXPORTER": "otlp",
			"OTEL_EXPORTER_OTLP_PROTOCOL": "grpc",
			"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:42442/otlp"
		}}`
		detector := telemetry.NewDetector(42442, writeSettings(t, grpcSettings))

		view := newTelemetryTimeView(claudeSession(), detector, telemetry.NewStore(), nil)

		assert.Equal(t, telemetry.ExportMisconfigured, view.Status)
		assert.Contains(t, view.Detail, "grpc")
	})

	// codex-session-nil
	t.Run("codex-session-nil", func(t *testing.T) {
		s := &session.Session{Agent: session.AgentCodex, Meta: session.Meta{SessionId: "s1"}}
		detector := telemetry.NewDetector(42442, writeSettings(t, configuredSettings))

		assert.Nil(t, newTelemetryTimeView(s, detector, telemetry.NewStore(), nil))
	})

	// nil-store-detector-fallback
	t.Run("nil-store-detector-fallback", func(t *testing.T) {
		detector := telemetry.NewDetector(42442, writeSettings(t, configuredSettings))

		view := newTelemetryTimeView(claudeSession(), detector, nil, nil)

		assert.Equal(t, telemetry.ExportConfigured, view.Status)
	})

	// nil-detector-nil
	t.Run("nil-detector-nil", func(t *testing.T) {
		assert.Nil(t, newTelemetryTimeView(claudeSession(), nil, telemetry.NewStore(), nil))
	})

	// nil-store-persisted-fallback
	t.Run("nil-store-persisted-fallback", func(t *testing.T) {
		dir := state.NewDir(t.TempDir())
		require.NoError(t, dir.WriteTelemetry("claude", "s1", `{"active_seconds":12.7,"cost_usd":0.5,"updated_at":"2026-08-13T09:00:00Z"}`))

		view := newTelemetryTimeView(claudeSession(), nil, nil, dir)

		assert.Equal(t, telemetry.ExportReceiving, view.Status)
		assert.Equal(t, "persisted", view.Detail)
		assert.Equal(t, 12, view.ActiveSeconds)
		assert.Equal(t, 0.5, view.CostUSD)
	})

	// live-store-wins-over-persisted
	t.Run("live-store-wins-over-persisted", func(t *testing.T) {
		store := telemetry.NewStore()
		payload := `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"claude_code.active_time.total","sum":{"aggregationTemporality":1,"dataPoints":[{"attributes":[{"key":"session.id","value":{"stringValue":"s1"}}],"asDouble":42.5}]}}]}]}]}`
		require.NoError(t, store.IngestMetrics([]byte(payload)))
		dir := state.NewDir(t.TempDir())
		require.NoError(t, dir.WriteTelemetry("claude", "s1", `{"active_seconds":1,"cost_usd":9.9,"updated_at":"2026-08-13T09:00:00Z"}`))

		view := newTelemetryTimeView(claudeSession(), nil, store, dir)

		assert.Equal(t, telemetry.ExportReceiving, view.Status)
		assert.Empty(t, view.Detail)
		assert.Equal(t, 42, view.ActiveSeconds)
	})

	// nil-store-no-file-nil-detector-nil
	t.Run("nil-store-no-file-nil-detector-nil", func(t *testing.T) {
		dir := state.NewDir(t.TempDir())

		assert.Nil(t, newTelemetryTimeView(claudeSession(), nil, nil, dir))
	})
}
