package telemetry

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetector_Status(t *testing.T) {
	type testCase struct {
		_id             string
		_detailContains string
		_state          ExportState
		settings        string
	}

	setupOutput := `{
  "env": {
    "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
    "OTEL_METRICS_EXPORTER": "otlp",
    "OTEL_EXPORTER_OTLP_PROTOCOL": "http/json",
    "OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:42442/otlp",
    "OTEL_METRIC_EXPORT_INTERVAL": "10000"
  }
}`

	tests := make([]*testCase, 0)

	// missing-file-not-configured
	tests = append(tests, &testCase{
		_id:             "missing-file-not-configured",
		_detailContains: "may still be enabled via shell env",
		_state:          ExportNotConfigured,
	})

	// invalid-json-not-configured
	tests = append(tests, &testCase{
		_id:             "invalid-json-not-configured",
		_detailContains: "cannot parse",
		_state:          ExportNotConfigured,
		settings:        "{not json",
	})

	// no-env-block-not-configured
	tests = append(tests, &testCase{
		_id:             "no-env-block-not-configured",
		_detailContains: "may still be enabled via shell env",
		_state:          ExportNotConfigured,
		settings:        `{"model": "opus"}`,
	})

	// enabled-zero-not-configured
	tests = append(tests, &testCase{
		_id:             "enabled-zero-not-configured",
		_detailContains: "CLAUDE_CODE_ENABLE_TELEMETRY=0",
		_state:          ExportNotConfigured,
		settings:        `{"env": {"CLAUDE_CODE_ENABLE_TELEMETRY": "0"}}`,
	})

	// exact-setup-output-configured
	tests = append(tests, &testCase{
		_id:      "exact-setup-output-configured",
		_state:   ExportConfigured,
		settings: setupOutput,
	})

	// localhost-endpoint-configured
	tests = append(tests, &testCase{
		_id:    "localhost-endpoint-configured",
		_state: ExportConfigured,
		settings: `{"env": {
			"CLAUDE_CODE_ENABLE_TELEMETRY": "true",
			"OTEL_METRICS_EXPORTER": "otlp",
			"OTEL_EXPORTER_OTLP_PROTOCOL": "http/json",
			"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:42442/otlp"
		}}`,
	})

	// trailing-slash-endpoint-configured
	tests = append(tests, &testCase{
		_id:    "trailing-slash-endpoint-configured",
		_state: ExportConfigured,
		settings: `{"env": {
			"CLAUDE_CODE_ENABLE_TELEMETRY": "1",
			"OTEL_METRICS_EXPORTER": "otlp",
			"OTEL_EXPORTER_OTLP_PROTOCOL": "http/json",
			"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:42442/otlp/"
		}}`,
	})

	// grpc-protocol-misconfigured
	tests = append(tests, &testCase{
		_id:             "grpc-protocol-misconfigured",
		_detailContains: `protocol "grpc" (want http/json)`,
		_state:          ExportMisconfigured,
		settings: `{"env": {
			"CLAUDE_CODE_ENABLE_TELEMETRY": "1",
			"OTEL_METRICS_EXPORTER": "otlp",
			"OTEL_EXPORTER_OTLP_PROTOCOL": "grpc",
			"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:4317"
		}}`,
	})

	// wrong-port-misconfigured
	tests = append(tests, &testCase{
		_id:             "wrong-port-misconfigured",
		_detailContains: "want http://127.0.0.1:42442/otlp",
		_state:          ExportMisconfigured,
		settings: `{"env": {
			"CLAUDE_CODE_ENABLE_TELEMETRY": "1",
			"OTEL_METRICS_EXPORTER": "otlp",
			"OTEL_EXPORTER_OTLP_PROTOCOL": "http/json",
			"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:42443/otlp"
		}}`,
	})

	// missing-endpoint-misconfigured
	tests = append(tests, &testCase{
		_id:             "missing-endpoint-misconfigured",
		_detailContains: "no OTEL_EXPORTER_OTLP_ENDPOINT",
		_state:          ExportMisconfigured,
		settings: `{"env": {
			"CLAUDE_CODE_ENABLE_TELEMETRY": "1",
			"OTEL_METRICS_EXPORTER": "otlp",
			"OTEL_EXPORTER_OTLP_PROTOCOL": "http/json"
		}}`,
	})

	// numeric-env-value-tolerated
	tests = append(tests, &testCase{
		_id:             "numeric-env-value-tolerated",
		_detailContains: "CLAUDE_CODE_ENABLE_TELEMETRY=0",
		_state:          ExportNotConfigured,
		settings:        `{"env": {"CLAUDE_CODE_ENABLE_TELEMETRY": 0}}`,
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if test.settings != "" {
				require.NoError(t, os.WriteFile(path, []byte(test.settings), 0o600))
			}

			status := NewDetector(42442, path).Status()

			assert.Equal(t, test._state, status.State)
			if test._detailContains != "" {
				assert.Contains(t, status.Detail, test._detailContains)
			}
			if test._state == ExportConfigured {
				assert.Empty(t, status.Detail)
			}
		})
	}
}

func TestDetector_Status_BoundPortMismatchDetail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	settings := `{"env": {
		"CLAUDE_CODE_ENABLE_TELEMETRY": "1",
		"OTEL_METRICS_EXPORTER": "otlp",
		"OTEL_EXPORTER_OTLP_PROTOCOL": "http/json",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:42442/otlp"
	}}`
	require.NoError(t, os.WriteFile(path, []byte(settings), 0o600))

	status := NewDetector(42443, path).Status()

	assert.Equal(t, ExportMisconfigured, status.State)
	assert.Contains(t, status.Detail, fmt.Sprintf("want http://127.0.0.1:%d/otlp", 42443))
}
