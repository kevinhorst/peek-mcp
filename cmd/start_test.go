package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/config"
	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newConfigFlagCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("back-link", "", "")
	cmd.Flags().Int("depth", 20, "")
	cmd.Flags().Duration("poll-interval", time.Second*5, "")
	cmd.Flags().Duration("poll-window", time.Hour, "")
	cmd.Flags().Int("state-retention-days", 90, "")
	cmd.Flags().String("log-level", "info", "")
	return cmd
}

func TestApplyConfigFileFallbacks(t *testing.T) {
	// flag-beats-file
	t.Run("flag-beats-file", func(t *testing.T) {
		cmd := newConfigFlagCommand()
		require.NoError(t, cmd.Flags().Set("depth", "30"))
		file := &config.File{}
		require.NoError(t, file.Set(config.KeyDepth, "50"))

		applyConfigFileFallbacks(cmd, file)

		depth, _ := cmd.Flags().GetInt("depth")
		assert.Equal(t, 30, depth)
	})

	// env-marked-changed-beats-file
	t.Run("env-marked-changed-beats-file", func(t *testing.T) {
		cmd := newConfigFlagCommand()
		require.NoError(t, cmd.Flags().Set("log-level", "warn"))
		file := &config.File{}
		require.NoError(t, file.Set(config.KeyLogLevel, "debug"))

		applyConfigFileFallbacks(cmd, file)

		level, _ := cmd.Flags().GetString("log-level")
		assert.Equal(t, "warn", level)
	})

	// file-beats-default
	t.Run("file-beats-default", func(t *testing.T) {
		cmd := newConfigFlagCommand()
		file := &config.File{}
		require.NoError(t, file.Set(config.KeyBackLink, "http://127.0.0.1:6001/"))
		require.NoError(t, file.Set(config.KeyDepth, "50"))
		require.NoError(t, file.Set(config.KeyPollInterval, "10s"))

		applyConfigFileFallbacks(cmd, file)

		backLink, _ := cmd.Flags().GetString("back-link")
		depth, _ := cmd.Flags().GetInt("depth")
		interval, _ := cmd.Flags().GetDuration("poll-interval")
		assert.Equal(t, "http://127.0.0.1:6001/", backLink)
		assert.Equal(t, 50, depth)
		assert.Equal(t, 10*time.Second, interval)
	})

	// empty-file-keeps-defaults
	t.Run("empty-file-keeps-defaults", func(t *testing.T) {
		cmd := newConfigFlagCommand()

		applyConfigFileFallbacks(cmd, &config.File{})

		depth, _ := cmd.Flags().GetInt("depth")
		level, _ := cmd.Flags().GetString("log-level")
		assert.Equal(t, 20, depth)
		assert.Equal(t, "info", level)
	})
}

func TestChangedConfigKeys(t *testing.T) {
	cmd := newConfigFlagCommand()
	require.NoError(t, cmd.Flags().Set("depth", "30"))

	changed := changedConfigKeys(cmd)

	assert.True(t, changed[config.KeyDepth])
	assert.False(t, changed[config.KeyBackLink])
	assert.False(t, changed[config.KeyLogLevel])
}

func serveHealthz(t *testing.T, store *session.Store) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	healthzHandler("/home/a/.claude", "/home/a/.codex", 42443, store)(rec, req)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return rec, body
}

func TestHealthzHandler(t *testing.T) {
	store := session.NewStore(10, 25, events.NewBroker())

	// ready-false-before-mark
	rec, body := serveHealthz(t, store)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, Version(), body["version"])
	assert.Equal(t, "/home/a/.claude", body["claudeHome"])
	assert.Equal(t, "/home/a/.codex", body["codexHome"])
	assert.Equal(t, float64(42443), body["controlPort"])
	assert.Equal(t, false, body["ready"])

	// ready-true-after-mark
	store.MarkReady()
	rec, body = serveHealthz(t, store)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, true, body["ready"])
}

func TestAwaitInitialLoad(t *testing.T) {
	type testCase struct {
		_expectedReady bool
		_id            string

		ctx   context.Context
		loads []<-chan struct{}
		store *session.Store
	}

	closedLoad := make(chan struct{})
	close(closedLoad)
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	tests := make([]*testCase, 0)

	// all-loaders-closed-marks-ready
	tests = append(tests, &testCase{
		_id:            "all-loaders-closed-marks-ready",
		_expectedReady: true,

		ctx:   context.Background(),
		loads: []<-chan struct{}{closedLoad, closedLoad},
		store: session.NewStore(10, 25, events.NewBroker()),
	})

	// no-loaders-marks-ready
	tests = append(tests, &testCase{
		_id:            "no-loaders-marks-ready",
		_expectedReady: true,

		ctx:   context.Background(),
		store: session.NewStore(10, 25, events.NewBroker()),
	})

	// cancelled-context-leaves-not-ready
	tests = append(tests, &testCase{
		_id:            "cancelled-context-leaves-not-ready",
		_expectedReady: false,

		ctx:   cancelledCtx,
		loads: []<-chan struct{}{closedLoad, make(chan struct{})},
		store: session.NewStore(10, 25, events.NewBroker()),
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			awaitInitialLoad(test.ctx, test.store, test.loads, time.Now())

			assert.Equal(t, test._expectedReady, test.store.IsReady())
		})
	}
}
