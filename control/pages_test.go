package control

import (
	"net/http"
	"os"
	"path/filepath"
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
	assert.Contains(t, body, `hx-get="/fragments/config"`)
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
	assert.Contains(t, body, `<table class="evidence-table">`)
	assert.NotContains(t, body, "<th>Transport</th>")
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
	assert.Contains(t, body, "<th>Total tokens</th><td>15</td>")
	assert.Contains(t, body, "<th>Plan versions</th><td>1</td>")
	assert.NotContains(t, body, "Plan alterations")
	assert.NotContains(t, body, "Cached input")
	assert.NotContains(t, body, "Reasoning output")

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
	assert.NotContains(t, body, ">Hub<")
}

func TestSessionsPage_BackLink(t *testing.T) {
	store, broker := newTestStore()
	server, err := New(&Options{
		Store:   store,
		Broker:  broker,
		Version: "test",
		Depth:   10,
		Config:  Config{BackLink: "http://127.0.0.1:6001/"},
	})
	require.NoError(t, err)

	response := get(server, "/")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `<a href="http://127.0.0.1:6001/">Hub</a>`)
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
	assert.Contains(t, body, `hx-get="/fragments/sessions/s1/memory"`)
	assert.Equal(t, 6, strings.Count(body, `<details class="section">`))
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
	assert.NotContains(t, body, `class="tabs subtabs"`)
}

func TestTurnsFragment_SubagentTabs(t *testing.T) {
	store, broker := newTestStore()
	now := time.Now()
	store.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		SubagentId: "ag1",
		Events: []*session.Event{{
			Kind:      session.EventKindSubagentSpawned,
			Actor:     "ag1",
			Subagent:  &session.SubagentPayload{AgentId: "ag1", AgentType: "Explore", Description: "scan"},
			Timestamp: now,
		}},
		Timestamp: now,
		Meta:      &session.Meta{SessionId: "s1"},
	})
	store.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		SubagentId: "ag1", Role: session.RoleUser, Text: "sub prompt", Timestamp: now, Meta: &session.Meta{SessionId: "s1"},
	})
	server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
	require.NoError(t, err)

	response := get(server, "/fragments/sessions/s1/turns")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, `class="tabs subtabs"`)
	assert.Contains(t, body, `?subagent=ag1`)
	assert.Contains(t, body, ">Explore</a>")
	assert.Contains(t, body, "What does this do?")
	assert.NotContains(t, body, "sub prompt")

	response = get(server, "/fragments/sessions/s1/turns?subagent=ag1")
	require.Equal(t, http.StatusOK, response.Code)
	body = response.Body.String()
	assert.Contains(t, body, "sub prompt")
	assert.NotContains(t, body, "What does this do?")
	assert.Contains(t, body, `class="active" title="scan">Explore</a>`)
}

func TestTurnsFragment_Thinking(t *testing.T) {
	store, broker := newTestStore()
	now := time.Now()
	store.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		Role: session.RoleAssistant, Text: "answer", Thinking: "reasoning here", RequestId: "r-think", Timestamp: now, Meta: &session.Meta{SessionId: "s1"},
	})
	store.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		Role: session.RoleUser, Text: "next prompt", Timestamp: now, Meta: &session.Meta{SessionId: "s1"},
	})
	server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
	require.NoError(t, err)

	response := get(server, "/fragments/sessions/s1/turns")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, `class="snippet thinking"`)
	assert.Contains(t, body, "reasoning here")
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

func TestMemoryFragment(t *testing.T) {
	// codex-unavailable
	t.Run("codex-unavailable", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s2/memory")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "memory is not available for codex sessions")
	})

	// no-path-unavailable
	t.Run("no-path-unavailable", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/memory")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "transcript path unknown")
	})

	// facts-rendered
	t.Run("facts-rendered", func(t *testing.T) {
		store, broker := newTestStore()
		projectDir := t.TempDir()
		memoryDir := filepath.Join(projectDir, "memory")
		require.NoError(t, os.Mkdir(memoryDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "MEMORY.md"), []byte("# Memory\n\n- [Likes Go](user_likes-go.md)"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "user_likes-go.md"), []byte("---\nname: likes-go\nmetadata:\n  type: user\n---\n\nLikes Go."), 0o644))
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.FilePath = filepath.Join(projectDir, "x.jsonl")
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/memory")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<h1>Memory</h1>")
		assert.Contains(t, body, "user_likes-go")
		assert.Contains(t, body, `<span class="badge">user</span>`)
		assert.Contains(t, body, "Likes Go.")
	})

	// not-found-404
	t.Run("not-found-404", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		assert.Equal(t, http.StatusNotFound, get(server, "/fragments/sessions/unknown/memory").Code)
	})
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
