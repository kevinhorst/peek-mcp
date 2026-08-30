package control

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinhorst/peek-mcp/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func post(server *Server, path string, form url.Values) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4243"+path, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func newConfigTestServer(t *testing.T, mutate ...func(*Options)) (*Server, string) {
	store, broker := newTestStore()
	configPath := filepath.Join(t.TempDir(), "config.json")
	opts := &Options{
		Store:      store,
		Broker:     broker,
		Version:    "test",
		Depth:      10,
		ConfigPath: configPath,
		Config: Config{
			Transport:          "http",
			Port:               4242,
			Depth:              20,
			PollInterval:       "5s",
			PollWindow:         "1h0m0s",
			StateRetentionDays: 90,
			ControlPort:        42442,
			LogLevel:           "info",
		},
	}
	for _, fn := range mutate {
		fn(opts)
	}
	server, err := New(opts)
	require.NoError(t, err)
	return server, configPath
}

func TestConfigFragment(t *testing.T) {
	// renders-card-config-rows
	t.Run("renders-card-config-rows", func(t *testing.T) {
		server, _ := newConfigTestServer(t)

		response := get(server, "/fragments/config")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, `class="card card-config" id="config-depth"`)
		assert.Contains(t, body, `hx-post="/api/config/depth"`)
		assert.Contains(t, body, `hx-post="/api/config/back-link"`)
	})

	// read-only-transport-no-form
	t.Run("read-only-transport-no-form", func(t *testing.T) {
		server, _ := newConfigTestServer(t)

		body := get(server, "/fragments/config").Body.String()
		assert.NotContains(t, body, `hx-post="/api/config/transport"`)
		assert.Contains(t, body, `id="config-transport"`)
	})

	// restart-required-badge-on-drift
	t.Run("restart-required-badge-on-drift", func(t *testing.T) {
		server, configPath := newConfigTestServer(t)
		file := &config.File{}
		require.NoError(t, file.Set(config.KeyDepth, "50"))
		require.NoError(t, config.Save(configPath, file))

		body := get(server, "/fragments/config").Body.String()
		assert.Contains(t, body, "restart required")
		assert.Contains(t, body, `value="50"`)
	})

	// overridden-badge
	t.Run("overridden-badge", func(t *testing.T) {
		server, _ := newConfigTestServer(t, func(opts *Options) {
			opts.OverriddenKeys = map[string]bool{config.KeyDepth: true}
		})

		body := get(server, "/fragments/config").Body.String()
		assert.Contains(t, body, "overridden")
	})

	// no-restart-button-without-closure
	t.Run("no-restart-button-without-closure", func(t *testing.T) {
		server, _ := newConfigTestServer(t)

		body := get(server, "/fragments/config").Body.String()
		assert.NotContains(t, body, "Restart peek-mcp?")
	})
}

func TestConfigSet(t *testing.T) {
	// valid-depth-writes-file-and-triggers
	t.Run("valid-depth-writes-file-and-triggers", func(t *testing.T) {
		server, configPath := newConfigTestServer(t)

		response := post(server, "/api/config/depth", url.Values{"value": {"200"}})
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "config-op", response.Header().Get("HX-Trigger"))
		assert.Contains(t, response.Body.String(), `id="config-depth"`)

		file, err := config.Load(configPath)
		require.NoError(t, err)
		require.NotNil(t, file.Depth)
		assert.Equal(t, 200, *file.Depth)
	})

	// invalid-value-400
	t.Run("invalid-value-400", func(t *testing.T) {
		server, configPath := newConfigTestServer(t)

		response := post(server, "/api/config/depth", url.Values{"value": {"abc"}})
		require.Equal(t, http.StatusBadRequest, response.Code)
		assert.Contains(t, response.Body.String(), "Invalid field depth")

		file, err := config.Load(configPath)
		require.NoError(t, err)
		assert.Nil(t, file.Depth)
	})

	// unknown-key-400
	t.Run("unknown-key-400", func(t *testing.T) {
		server, _ := newConfigTestServer(t)

		response := post(server, "/api/config/nonsense", url.Values{"value": {"1"}})
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	// non-editable-key-400
	t.Run("non-editable-key-400", func(t *testing.T) {
		server, _ := newConfigTestServer(t)

		response := post(server, "/api/config/transport", url.Values{"value": {"stdio"}})
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
