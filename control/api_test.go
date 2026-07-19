package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
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
