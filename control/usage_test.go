package control

import (
	"net/http"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisplayTotalTokens(t *testing.T) {
	// codex-total-preferred
	assert.Equal(t, 100, displayTotalTokens(&session.Usage{TotalTokens: 100, InputTokens: 10, OutputTokens: 5}))
	// claude-summed
	assert.Equal(t, 40, displayTotalTokens(&session.Usage{InputTokens: 10, OutputTokens: 5, CacheCreationInputTokens: 10, CacheReadInputTokens: 15}))
}

func TestCachePercent(t *testing.T) {
	// claude-read-share
	assert.Equal(t, "75%", cachePercent(session.AgentClaude, &session.Usage{InputTokens: 10, CacheCreationInputTokens: 15, CacheReadInputTokens: 75}))
	// codex-cached-share
	assert.Equal(t, "50%", cachePercent(session.AgentCodex, &session.Usage{InputTokens: 100, CachedInputTokens: 50}))
	// empty-base-blank
	assert.Equal(t, "", cachePercent(session.AgentClaude, &session.Usage{}))
}

func TestUsageCostFragment(t *testing.T) {
	// known-model
	t.Run("known-model", func(t *testing.T) {
		store, broker := newTestStore()
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.Meta.Model = "claude-fable-5"
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage/cost")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>Total</th>")
		assert.Contains(t, body, "Estimate from embedded rates for claude-fable-5")
	})

	// unknown-model
	t.Run("unknown-model", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage/cost")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "No pricing for model opus")
	})

	// not-found-404
	t.Run("not-found-404", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		assert.Equal(t, http.StatusNotFound, get(server, "/fragments/sessions/unknown/usage/cost").Code)
	})
}

func TestUsagePlansFragment(t *testing.T) {
	// rows-rendered
	t.Run("rows-rendered", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage/plans")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>Revision</th>")
		assert.Contains(t, body, "<th>0</th>")
	})

	// empty-state
	t.Run("empty-state", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s2/usage/plans")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "No plan versions yet.")
	})
}

func TestUsageSkillsFragment(t *testing.T) {
	// rows-rendered
	t.Run("rows-rendered", func(t *testing.T) {
		store, broker := newTestStore()
		started := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.Skills = append(sess.Skills, &session.SkillStat{
				Skill:     "jq",
				StartedAt: started,
				EndedAt:   started.Add(90 * time.Second),
				Usage:     session.Usage{InputTokens: 10, OutputTokens: 20},
			})
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage/skills")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>jq</th>")
		assert.Contains(t, body, "1m30s")
		assert.Contains(t, body, "<td>30</td>")
	})

	// empty-state
	t.Run("empty-state", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage/skills")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "No skills invoked yet.")
	})
}

func TestUsageDenialsFragment(t *testing.T) {
	// rows-with-command
	t.Run("rows-with-command", func(t *testing.T) {
		store, broker := newTestStore()
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.AddEvent(&session.Event{
				Kind:       session.EventKindPermissionDenied,
				Permission: &session.PermissionPayload{Tool: "Bash", Command: "rm -rf /tmp/x"},
				Timestamp:  time.Now(),
			})
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage/denials")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>Bash</th>")
		assert.Contains(t, body, "rm -rf /tmp/x")
	})

	// empty-state
	t.Run("empty-state", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage/denials")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "No permission denials.")
	})
}
