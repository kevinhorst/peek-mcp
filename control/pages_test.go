package control

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsPage(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/stats")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, `href="/stats"`)
	assert.Contains(t, body, `hx-get="/fragments/stats"`)
}

func TestStatsFragment(t *testing.T) {
	store, broker := newTestStore()
	server, err := New(&Options{
		Store:   store,
		Broker:  broker,
		Version: "test",
		Depth:   10,
		Config:  Config{Transport: "http", ControlPort: 4243},
	})
	require.NoError(t, err)

	response := get(server, "/fragments/stats")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "<th>PID</th>")
	assert.Contains(t, body, "<th>Transport</th>")
	assert.Contains(t, body, "1 claude · 1 codex · 2 total")
	assert.NotContains(t, body, "Restart")
}

func TestUsageFragment(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/fragments/sessions/s1/usage")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "<th>Input tokens</th><td>10</td>")
	assert.Contains(t, body, "<th>Skills invoked</th><td>0</td>")

	assert.Equal(t, http.StatusNotFound, get(server, "/fragments/sessions/unknown/usage").Code)
}

func TestEventsFragment(t *testing.T) {
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

	response := get(server, "/fragments/sessions/s1/events")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "skill_invoked")
	assert.Contains(t, body, "peek")

	response = get(server, "/fragments/sessions/s2/events")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "No events yet.")
}

func TestSessionsPage(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "agent=claude")
	assert.Contains(t, body, "agent=codex")
	assert.Contains(t, body, "htmx.min.js")
}

func TestSessionsFragment(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/fragments/sessions?agent=claude")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "evidence-table")
	assert.Contains(t, body, "Login simplification")
	assert.Contains(t, body, `href="/sessions/s1"`)
	assert.NotContains(t, body, `href="/sessions/s2"`)
	assert.NotContains(t, body, `<span class="badge">claude`)
	assert.Contains(t, body, `id="last-active-claude"`)
}

func TestSessionsFragment_MissingAgent(t *testing.T) {
	server, _ := newTestServer(t, "")

	assert.Equal(t, http.StatusBadRequest, get(server, "/fragments/sessions").Code)
	assert.Equal(t, http.StatusBadRequest, get(server, "/fragments/sessions?agent=bogus").Code)
}

func TestSessionsFragment_Pagination(t *testing.T) {
	broker := events.NewBroker()
	store := session.NewStore(10, broker, session.AgentClaude)
	for i := 0; i < defaultSessionLimit+1; i++ {
		id := session.Id("p" + strconv.Itoa(i))
		store.AddTurnBySessionId(id, session.AgentClaude, &session.Turn{
			Role:      session.RoleUser,
			Text:      "hi",
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
			Meta:      &session.Meta{SessionId: id},
		})
	}
	server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
	require.NoError(t, err)

	page1 := get(server, "/fragments/sessions?agent=claude&offset=0")
	require.Equal(t, http.StatusOK, page1.Code)
	assert.Equal(t, defaultSessionLimit, strings.Count(page1.Body.String(), `href="/sessions/`))
	assert.Contains(t, page1.Body.String(), "offset=50")

	page2 := get(server, "/fragments/sessions?agent=claude&offset=50")
	require.Equal(t, http.StatusOK, page2.Code)
	assert.Equal(t, 1, strings.Count(page2.Body.String(), `href="/sessions/`))
	assert.Contains(t, page2.Body.String(), ">prev<")
}

func TestSessionsFragment_Empty(t *testing.T) {
	broker := events.NewBroker()
	store := session.NewStore(10, broker)
	server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
	require.NoError(t, err)

	response := get(server, "/fragments/sessions?agent=codex")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "No codex sessions yet.")
}

func TestSessionDetailPage(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := get(server, "/sessions/s1")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "Login simplification")
	assert.Contains(t, body, `hx-get="/fragments/sessions/s1/turns"`)
	assert.Contains(t, body, `hx-get="/fragments/sessions/s1/plan"`)
	assert.Contains(t, body, `hx-get="/fragments/sessions/s1/usage"`)
	assert.Contains(t, body, `hx-get="/fragments/sessions/s1/events"`)
	assert.Equal(t, 5, strings.Count(body, `<details class="section">`))
	assert.Equal(t, 1, strings.Count(body, `<details class="section" open>`))

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
