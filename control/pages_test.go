package control

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionsPage(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `hx-get="/fragments/sessions"`)
	assert.Contains(t, response.Body.String(), "htmx.min.js")
}

func TestSessionsFragment(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/fragments/sessions")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "Login simplification")
	assert.Contains(t, body, `href="/sessions/s1"`)
	assert.Contains(t, body, `href="/sessions/s2"`)
}

func TestSessionsFragment_Empty(t *testing.T) {
	broker := events.NewBroker()
	store := session.NewStore(10, broker)
	server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
	require.NoError(t, err)

	response := get(server, "/fragments/sessions")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "No sessions yet")
}

func TestSessionDetailPage(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/sessions/s1")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "Login simplification")
	assert.Contains(t, body, `hx-get="/fragments/sessions/s1/turns"`)
	assert.Contains(t, body, `hx-get="/fragments/sessions/s1/plan"`)

	assert.Equal(t, http.StatusNotFound, get(server, "/sessions/unknown").Code)
}

func TestTurnsFragment(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/fragments/sessions/s1/turns")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "What does this do?")
	assert.Contains(t, body, "It does things.")
}

func TestPlanFragment(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/fragments/sessions/s1/plan")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "<h1>Plan</h1>")

	response = get(server, "/fragments/sessions/s2/plan")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "No plan.")
}

func TestDiffFragment(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/fragments/sessions/s1/diff")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "diff-content")
	assert.Contains(t, body, "vs main")
	assert.NotContains(t, body, "truncated")

	response = get(server, "/fragments/sessions/s2/diff")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "No diff.")
}

func TestDiffFragment_Truncated(t *testing.T) {
	store, broker := newTestStore()
	store.UpdateDiff("s1", "main", strings.Repeat("x", defaultDiffSize+1))
	server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
	require.NoError(t, err)

	response := get(server, "/fragments/sessions/s1/diff")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "truncated at 256 KB")
}
