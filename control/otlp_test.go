package control

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kevinhorst/peek-mcp/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const otlpTestPayload = `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"claude_code.active_time.total","sum":{"aggregationTemporality":1,"dataPoints":[{"attributes":[{"key":"session.id","value":{"stringValue":"s1"}}],"asDouble":42.5}]}}]}]}]}`

func postOtlp(server *Server, body, contentType string, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4243/otlp/v1/metrics", strings.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	for _, fn := range mutate {
		fn(request)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func newTelemetryTestServer(t *testing.T, token string) (*Server, *telemetry.Store) {
	store, broker := newTestStore()
	telemetryStore := telemetry.NewStore()
	server, err := New(&Options{Store: store, Broker: broker, Telemetry: telemetryStore, Token: token, Version: "test", Depth: 10})
	require.NoError(t, err)
	return server, telemetryStore
}

func TestHandleOtlpMetrics(t *testing.T) {
	// valid-payload-folded
	t.Run("valid-payload-folded", func(t *testing.T) {
		server, telemetryStore := newTelemetryTestServer(t, "")
		response := postOtlp(server, otlpTestPayload, "application/json")

		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "{}", response.Body.String())
		stats, ok := telemetryStore.Get("s1")
		require.True(t, ok)
		assert.Equal(t, 42.5, stats.ActiveSeconds)
	})

	// protobuf-content-type-rejected
	t.Run("protobuf-content-type-rejected", func(t *testing.T) {
		server, _ := newTelemetryTestServer(t, "")
		response := postOtlp(server, otlpTestPayload, "application/x-protobuf")
		assert.Equal(t, http.StatusUnsupportedMediaType, response.Code)
	})

	// invalid-payload-bad-request
	t.Run("invalid-payload-bad-request", func(t *testing.T) {
		server, _ := newTelemetryTestServer(t, "")
		response := postOtlp(server, "{not json", "application/json")
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	// token-required-when-set
	t.Run("token-required-when-set", func(t *testing.T) {
		server, _ := newTelemetryTestServer(t, "secret")
		response := postOtlp(server, otlpTestPayload, "application/json")
		assert.Equal(t, http.StatusUnauthorized, response.Code)

		authorized := postOtlp(server, otlpTestPayload, "application/json", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer secret")
		})
		assert.Equal(t, http.StatusOK, authorized.Code)
	})

	// telemetry-disabled-not-found
	t.Run("telemetry-disabled-not-found", func(t *testing.T) {
		server, _ := newTestServer(t, "")
		response := postOtlp(server, otlpTestPayload, "application/json")
		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}
