package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/kevinhorst/peek-mcp/telemetry"
	"github.com/kevinhorst/peek-mcp/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decode[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	var v T
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &v))
	return v
}

func TestSessions(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/api/sessions")
	require.Equal(t, http.StatusOK, response.Code)
	list := decode[sessionsResponse](t, response)
	require.Len(t, list.Sessions, 2)
	assert.Equal(t, 2, list.Total)
	assert.Equal(t, session.Id("s2"), list.Sessions[0].Id)
	assert.Equal(t, session.Id("s1"), list.Sessions[1].Id)
	assert.Equal(t, "Login simplification", list.Sessions[1].Title)
	assert.Equal(t, "/project", list.Sessions[1].CWD)
	assert.Equal(t, "main", list.Sessions[1].GitBranch)
	assert.Equal(t, "opus", list.Sessions[1].Model)
	assert.True(t, list.Sessions[1].HasPlan)
	assert.True(t, list.Sessions[1].HasDiff)
	assert.True(t, list.Sessions[1].HasUncommittedDiff)
	assert.False(t, list.Sessions[0].HasPlan)
}

func TestSessions_AgentFilter(t *testing.T) {
	server, _ := newTestServer(t, "")

	list := decode[sessionsResponse](t, get(server, "/api/sessions?agent=codex"))
	require.Len(t, list.Sessions, 1)
	assert.Equal(t, session.Id("s2"), list.Sessions[0].Id)

	response := get(server, "/api/sessions?agent=bogus")
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestSessions_TitleFilter(t *testing.T) {
	server, _ := newTestServer(t, "")

	list := decode[sessionsResponse](t, get(server, "/api/sessions?title=login"))
	require.Len(t, list.Sessions, 1)
	assert.Equal(t, session.Id("s1"), list.Sessions[0].Id)
}

func TestSessions_Limit(t *testing.T) {
	server, _ := newTestServer(t, "")

	list := decode[sessionsResponse](t, get(server, "/api/sessions?limit=1"))
	assert.Len(t, list.Sessions, 1)

	response := get(server, "/api/sessions?limit=-1")
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestSessions_Offset(t *testing.T) {
	server, _ := newTestServer(t, "")

	list := decode[sessionsResponse](t, get(server, "/api/sessions?offset=1"))
	require.Len(t, list.Sessions, 1)
	assert.Equal(t, 2, list.Total)
	assert.Equal(t, session.Id("s1"), list.Sessions[0].Id)

	beyond := decode[sessionsResponse](t, get(server, "/api/sessions?offset=5"))
	assert.Empty(t, beyond.Sessions)
	assert.Equal(t, 2, beyond.Total)

	assert.Equal(t, http.StatusBadRequest, get(server, "/api/sessions?offset=-1").Code)
	assert.Equal(t, http.StatusBadRequest, get(server, "/api/sessions?offset=x").Code)
}

func TestSessionDetail(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/api/sessions/s1")
	require.Equal(t, http.StatusOK, response.Code)
	detail := decode[sessionDetail](t, response)
	assert.Equal(t, session.Id("s1"), detail.Id)
	assert.Equal(t, "main", detail.DiffTarget)
	assert.Equal(t, 10, detail.TotalUsage.InputTokens)
	assert.Equal(t, 5, detail.TotalUsage.OutputTokens)

	assert.Equal(t, http.StatusNotFound, get(server, "/api/sessions/unknown").Code)
}

func TestTurns(t *testing.T) {
	server, _ := newTestServer(t, "")

	turns := decode[turnsResponse](t, get(server, "/api/sessions/s1/turns"))
	assert.Len(t, turns.Turns, 2)

	turns = decode[turnsResponse](t, get(server, "/api/sessions/s1/turns?n=1"))
	assert.Len(t, turns.Turns, 1)

	assert.Equal(t, http.StatusBadRequest, get(server, "/api/sessions/s1/turns?n=x").Code)
	assert.Equal(t, http.StatusNotFound, get(server, "/api/sessions/unknown/turns").Code)
}

func TestPlan(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/api/sessions/s1/plan")
	require.Equal(t, http.StatusOK, response.Code)
	plan := decode[planResponse](t, response)
	assert.Equal(t, "# Plan\n\ncontent", plan.PlanContent)
	assert.Equal(t, "/plans/p.md", plan.PlanFilePath)

	assert.Equal(t, http.StatusNotFound, get(server, "/api/sessions/s2/plan").Code)
	assert.Equal(t, http.StatusNotFound, get(server, "/api/sessions/unknown/plan").Code)
}

func TestDiff(t *testing.T) {
	server, _ := newTestServer(t, "")

	diff := decode[diffResponse](t, get(server, "/api/sessions/s1/diff"))
	assert.Equal(t, "main", diff.Target)
	assert.Equal(t, "diff-content", diff.Diff)
	assert.False(t, diff.Truncated)

	diff = decode[diffResponse](t, get(server, "/api/sessions/s1/diff?size=4"))
	assert.Equal(t, "diff", diff.Diff)
	assert.True(t, diff.Truncated)

	diff = decode[diffResponse](t, get(server, "/api/sessions/s1/diff?size=0"))
	assert.Equal(t, "diff-content", diff.Diff)
	assert.False(t, diff.Truncated)

	assert.Equal(t, http.StatusBadRequest, get(server, "/api/sessions/s1/diff?size=x").Code)
}

func TestUncommittedDiff(t *testing.T) {
	server, _ := newTestServer(t, "")

	diff := decode[diffResponse](t, get(server, "/api/sessions/s1/uncommitted-diff"))
	assert.Empty(t, diff.Target)
	assert.Equal(t, "uncommitted-content", diff.Diff)
}

func TestDiff_DefaultTruncation(t *testing.T) {
	store, broker := newTestStore()
	store.UpdateDiff("s1", "main", strings.Repeat("x", defaultDiffSize+1))
	server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
	require.NoError(t, err)

	diff := decode[diffResponse](t, get(server, "/api/sessions/s1/diff"))
	assert.Len(t, diff.Diff, defaultDiffSize)
	assert.True(t, diff.Truncated)
}

func TestUsage(t *testing.T) {
	server, _ := newTestServer(t, "")

	usage := decode[usageResponse](t, get(server, "/api/sessions/s1/usage"))
	assert.Equal(t, 10, usage.TotalUsage.InputTokens)

	assert.Equal(t, http.StatusNotFound, get(server, "/api/sessions/unknown/usage").Code)
}

func TestHealthz(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/api/healthz")
	require.Equal(t, http.StatusOK, response.Code)
	health := decode[healthzResponse](t, response)
	assert.Equal(t, "ok", health.Status)
	assert.Equal(t, "test", health.Version)
}

func TestStats(t *testing.T) {
	store, broker := newTestStore()
	stateDir := state.NewDir(t.TempDir())
	require.NoError(t, stateDir.WriteDiffSnapshot("claude", "diff content", "s1"))
	invocations := tools.NewInvocationCounter()
	invocations.Inc("session_list")
	server, err := New(&Options{
		Store:       store,
		Broker:      broker,
		Token:       "secret123",
		Version:     "test",
		Depth:       10,
		StartedAt:   time.Now().Add(-time.Minute),
		StateDir:    stateDir,
		Invocations: invocations,
		Config:      Config{Transport: "http", ControlPort: 4243, TokenSet: true},
	})
	require.NoError(t, err)

	response := get(server, "/api/stats", func(r *http.Request) { r.Header.Set("Authorization", "Bearer secret123") })
	require.Equal(t, http.StatusOK, response.Code)
	stats := decode[statsResponse](t, response)
	assert.Equal(t, os.Getpid(), stats.PID)
	assert.Equal(t, "test", stats.Version)
	assert.NotEmpty(t, stats.Uptime)
	assert.Positive(t, stats.Goroutines)
	assert.Equal(t, sessionCounts{Claude: 1, Codex: 1, Total: 2}, stats.Sessions)
	assert.Positive(t, stats.StateDiskBytes)
	assert.Equal(t, map[string]int{"session_list": 1}, stats.Invocations)
	assert.True(t, stats.Config.TokenSet)
	assert.NotContains(t, response.Body.String(), "secret123")
}

func TestStats_WithoutStateDir(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/api/stats")
	require.Equal(t, http.StatusOK, response.Code)
	assert.NotContains(t, response.Body.String(), "state_disk_bytes")
	assert.NotContains(t, response.Body.String(), "invocations")
}

func TestHandleStats_TelemetryExport(t *testing.T) {
	// detector-set-includes-status
	t.Run("detector-set-includes-status", func(t *testing.T) {
		store, broker := newTestStore()
		settingsPath := filepath.Join(t.TempDir(), "settings.json")
		server, err := New(&Options{
			Store:    store,
			Broker:   broker,
			Version:  "test",
			Depth:    10,
			Detector: telemetry.NewDetector(42442, settingsPath),
		})
		require.NoError(t, err)

		response := get(server, "/api/stats")
		require.Equal(t, http.StatusOK, response.Code)
		stats := decode[statsResponse](t, response)
		require.NotNil(t, stats.TelemetryExport)
		assert.Equal(t, telemetry.ExportNotConfigured, stats.TelemetryExport.State)
	})

	// detector-nil-omits-field
	t.Run("detector-nil-omits-field", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/api/stats")
		require.Equal(t, http.StatusOK, response.Code)
		assert.NotContains(t, response.Body.String(), "telemetry_export")
	})
}

func TestSessionEvents(t *testing.T) {
	store, broker := newTestStore()
	store.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		Events: []*session.Event{{
			Kind:      session.EventKindSkillInvoked,
			Skill:     &session.SkillPayload{Skill: "peek", Source: session.SkillSourceSlash},
			Timestamp: time.Now(),
		}},
		Meta: &session.Meta{SessionId: "s1"},
	})
	server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
	require.NoError(t, err)

	response := get(server, "/api/sessions/s1/events")
	require.Equal(t, http.StatusOK, response.Code)
	resp := decode[eventsResponse](t, response)
	assert.Equal(t, 1, resp.Counters.SkillsInvoked)
	require.Len(t, resp.Events, 1)
	assert.Equal(t, session.EventKindSkillInvoked, resp.Events[0].Event)
	assert.Equal(t, "peek", resp.Events[0].Summary)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 1, resp.PlanRevisions)

	assert.Equal(t, http.StatusNotFound, get(server, "/api/sessions/unknown/events").Code)
}

func TestMemoryAPI(t *testing.T) {
	// claude-with-memory-200
	t.Run("claude-with-memory-200", func(t *testing.T) {
		store, broker := newTestStore()
		projectDir := t.TempDir()
		memoryDir := filepath.Join(projectDir, "memory")
		require.NoError(t, os.Mkdir(memoryDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "MEMORY.md"), []byte("# Memory"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "user_fact.md"), []byte("---\ntype: user\n---\n\nA fact."), 0o644))
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.FilePath = filepath.Join(projectDir, "x.jsonl")
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/api/sessions/s1/memory")
		require.Equal(t, http.StatusOK, response.Code)
		memory := decode[claude.Memory](t, response)
		assert.Equal(t, "# Memory", memory.Index)
		require.Len(t, memory.Facts, 1)
		assert.Equal(t, "user_fact", memory.Facts[0].Name)
		assert.Equal(t, "user", memory.Facts[0].Type)
	})

	// codex-404
	t.Run("codex-404", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		assert.Equal(t, http.StatusNotFound, get(server, "/api/sessions/s2/memory").Code)
	})
}
