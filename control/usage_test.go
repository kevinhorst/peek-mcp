package control

import (
	"net/http"
	"strings"
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

func TestUsageCostDetail(t *testing.T) {
	// known-model
	t.Run("known-model", func(t *testing.T) {
		store, broker := newTestStore()
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.Meta.Model = "claude-fable-5"
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage?detail=cost")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>Total</th>")
		assert.Contains(t, body, "Estimate from embedded rates (as of 2026-09-04) for claude-fable-5")
	})

	// tiered-rows
	t.Run("tiered-rows", func(t *testing.T) {
		store, broker := newTestStore()
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.Meta.Model = "claude-fable-5"
			sess.TotalUsage = session.Usage{CacheCreationInputTokens: 1100, CacheCreation5mInputTokens: 100, CacheCreation1hInputTokens: 1000}
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage?detail=cost")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>Cache write (5m)</th><td>100</td>")
		assert.Contains(t, body, "<th>Cache write (1h)</th><td>1000</td>")
	})

	// untiered-legacy
	t.Run("untiered-legacy", func(t *testing.T) {
		store, broker := newTestStore()
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.Meta.Model = "claude-fable-5"
			sess.TotalUsage = session.Usage{CacheCreationInputTokens: 200}
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage?detail=cost")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>Cache write (5m)</th><td>200</td>")
		assert.Contains(t, body, "<th>Cache write (1h)</th><td>0</td>")
	})

	// unknown-model
	t.Run("unknown-model", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage?detail=cost")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "No pricing for model opus")
	})

	// not-found-404
	t.Run("not-found-404", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		assert.Equal(t, http.StatusNotFound, get(server, "/fragments/sessions/unknown/usage?detail=cost").Code)
	})

	// old-route-404
	t.Run("old-route-404", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		assert.Equal(t, http.StatusNotFound, get(server, "/fragments/sessions/s1/usage/cost").Code)
	})

	// invalid-detail-plain-panel
	t.Run("invalid-detail-plain-panel", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage?detail=bogus")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>Input tokens</th>")
		assert.NotContains(t, body, "usage?detail=bogus")
	})
}

func TestUsagePlansDetail(t *testing.T) {
	// rows-rendered
	t.Run("rows-rendered", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage?detail=plans")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>Revision</th>")
		assert.Contains(t, body, "<th>Phase</th>")
		assert.Contains(t, body, "<th>0</th>")
		assert.Contains(t, body, "initial")
	})

	// empty-state
	t.Run("empty-state", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s2/usage?detail=plans")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "No plan versions yet.")
	})
}

func TestRevisionPhase(t *testing.T) {
	// initial
	assert.Equal(t, "initial", revisionPhase(&session.PlanRevision{Index: 0}))
	// planning
	assert.Equal(t, "planning", revisionPhase(&session.PlanRevision{Index: 1}))
	// alteration
	assert.Equal(t, "alteration", revisionPhase(&session.PlanRevision{Index: 2, IsAlteration: true}))
}

func TestRevisionDelta(t *testing.T) {
	// initial-line-count
	assert.Equal(t, "+2", revisionDelta(&session.PlanRevision{Index: 0, Content: "a\nb"}))
	// diff-counts
	assert.Equal(t, "+2 −1", revisionDelta(&session.PlanRevision{Index: 1, Diff: "--- previous\n+++ current\n+x\n+y\n-z\n context\n"}))
	// empty-diff-blank
	assert.Equal(t, "", revisionDelta(&session.PlanRevision{Index: 1, Diff: "--- previous\n+++ current\n"}))
	// truncated-over-cap
	added := strings.Repeat("+x\n", 1500)
	assert.Equal(t, "+999+ −1", revisionDelta(&session.PlanRevision{Index: 1, Diff: added + "-z\n"}))
}

func TestUsageSkillsDetail(t *testing.T) {
	// rows-rendered-with-cost
	t.Run("rows-rendered-with-cost", func(t *testing.T) {
		store, broker := newTestStore()
		started := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.Skills = append(sess.Skills, &session.SkillStat{
				Skill:     "jq",
				Model:     "claude-fable-5",
				StartedAt: started,
				EndedAt:   started.Add(90 * time.Second),
				Usage:     session.Usage{InputTokens: 10, OutputTokens: 20},
			})
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage?detail=skills")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>jq</th>")
		assert.Contains(t, body, "1m30s")
		assert.Contains(t, body, "<td>30</td>")
		assert.Contains(t, body, "<th>Cost</th>")
		assert.Contains(t, body, "$0.0")
	})

	// unknown-model-blank-cost
	t.Run("unknown-model-blank-cost", func(t *testing.T) {
		store, broker := newTestStore()
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.Skills = append(sess.Skills, &session.SkillStat{
				Skill:     "jq",
				StartedAt: time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC),
				Usage:     session.Usage{InputTokens: 10},
			})
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage?detail=skills")
		require.Equal(t, http.StatusOK, response.Code)
		assert.NotContains(t, response.Body.String(), "$")
	})

	// empty-state
	t.Run("empty-state", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage?detail=skills")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "No skills invoked yet.")
	})
}

func TestUsageSubagentsDetail(t *testing.T) {
	// rows-rendered-with-cost
	t.Run("rows-rendered-with-cost", func(t *testing.T) {
		store, broker := newTestStore()
		started := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.Meta.Model = "claude-fable-5"
			sess.Subagents = map[string]*session.SubagentStat{
				"a1": {
					AgentType:   "Explore",
					Description: "find things",
					Model:       "claude-haiku-4-5-20251001",
					FirstActive: started,
					LastActive:  started.Add(90 * time.Second),
					Usage:       session.Usage{InputTokens: 10, OutputTokens: 20},
				},
			}
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage?detail=subagents")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>Explore</th>")
		assert.Contains(t, body, "<td>find things</td>")
		assert.Contains(t, body, "<td>claude-haiku-4-5-20251001</td>")
		assert.Contains(t, body, "1m30s")
		assert.Contains(t, body, "<td>30</td>")
		assert.Contains(t, body, "$0.0")
	})

	// sorted-by-start
	t.Run("sorted-by-start", func(t *testing.T) {
		store, broker := newTestStore()
		started := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.Subagents = map[string]*session.SubagentStat{
				"a2": {AgentType: "later", FirstActive: started.Add(time.Minute), LastActive: started.Add(2 * time.Minute)},
				"a1": {AgentType: "earlier", FirstActive: started, LastActive: started.Add(time.Minute)},
			}
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage?detail=subagents")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Less(t, strings.Index(body, "<th>earlier</th>"), strings.Index(body, "<th>later</th>"))
	})

	// empty-state
	t.Run("empty-state", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage?detail=subagents")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "No subagents spawned yet.")
	})

	// row-clickable
	t.Run("row-clickable", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "usage?detail=subagents")
	})
}

func TestUsageFilesDetail(t *testing.T) {
	// rows-rendered
	t.Run("rows-rendered", func(t *testing.T) {
		store, broker := newTestStore()
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.AddFileTouch(&session.FileTouch{Path: "/b/second.go", Write: true})
			sess.AddFileTouch(&session.FileTouch{Path: "/b/second.go", Write: true})
			sess.AddFileTouch(&session.FileTouch{Path: "/a/first.go"})
			sess.AddFileTouch(&session.FileTouch{Path: "/a/first.go"})
			sess.AddFileTouch(&session.FileTouch{Path: "/a/first.go"})
			sess.AddFileTouch(&session.FileTouch{Path: "/a/first.go", Write: true})
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage?detail=files")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>/a/first.go</th>")
		assert.Contains(t, body, "<td>3</td>")
		assert.Less(t, strings.Index(body, "/a/first.go"), strings.Index(body, "/b/second.go"))
		assert.Contains(t, body, "<th>Touched files</th>")
		assert.Contains(t, body, "<td>2</td>")
	})

	// claude-config-split
	t.Run("claude-config-split", func(t *testing.T) {
		store, broker := newTestStore()
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.AddFileTouch(&session.FileTouch{Path: "/repo/.claude/worktrees/w/control/api.go"})
			sess.AddFileTouch(&session.FileTouch{Path: "/home/k/.claude/plans/x.md"})
			sess.AddFileTouch(&session.FileTouch{Path: "/repo/.claude/settings.json"})
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		body := get(server, "/fragments/sessions/s1/usage?detail=files").Body.String()
		assert.Contains(t, body, `<details class="section">`)
		assert.Contains(t, body, "<summary>.claude")
		// worktree file stays in the open list, above the .claude dropdown
		assert.Less(t, strings.Index(body, "worktrees/w/control/api.go"), strings.Index(body, "<summary>.claude"))
		// config files sit inside the dropdown, after its summary
		assert.Greater(t, strings.Index(body, "/home/k/.claude/plans/x.md"), strings.Index(body, "<summary>.claude"))
		assert.Greater(t, strings.Index(body, "/repo/.claude/settings.json"), strings.Index(body, "<summary>.claude"))
	})

	// only-config
	t.Run("only-config", func(t *testing.T) {
		store, broker := newTestStore()
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.AddFileTouch(&session.FileTouch{Path: "/home/k/.claude/plans/x.md"})
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		body := get(server, "/fragments/sessions/s1/usage?detail=files").Body.String()
		assert.Contains(t, body, "<summary>.claude")
		assert.NotContains(t, body, "No touched files.")
	})

	// empty-state
	t.Run("empty-state", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage?detail=files")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "No touched files.")
		assert.Contains(t, body, "<td>0</td>")
	})
}

func TestUsageTimeRows(t *testing.T) {
	// rows-rendered
	t.Run("rows-rendered", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>Session time</th>")
		assert.Contains(t, body, "<td>1m0s</td>")
		assert.Contains(t, body, "<th>Idle time</th>")
		assert.Contains(t, body, "<th>Active time</th>")
	})

	// no-started-at-hidden
	t.Run("no-started-at-hidden", func(t *testing.T) {
		store, broker := newTestStore()
		store.AddTurnBySessionId("s3", session.AgentClaude, &session.Turn{
			Role: session.RoleUser,
			Text: "untimed",
			Meta: &session.Meta{SessionId: "s3"},
		})
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s3/usage")
		require.Equal(t, http.StatusOK, response.Code)
		assert.NotContains(t, response.Body.String(), "<th>Session time</th>")
	})
}

func TestUsageModelsDetail(t *testing.T) {
	// rows-rendered
	t.Run("rows-rendered", func(t *testing.T) {
		store, broker := newTestStore()
		require.True(t, store.WithSession("s1", func(sess *session.Session) {
			sess.AddEvent(&session.Event{
				Kind:      session.EventKindModelChanged,
				Model:     &session.ModelPayload{From: "claude-opus-4-6", To: "claude-fable-5"},
				Timestamp: time.Now(),
			})
		}))
		server, err := New(&Options{Store: store, Broker: broker, Version: "test", Depth: 10})
		require.NoError(t, err)

		response := get(server, "/fragments/sessions/s1/usage?detail=models")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<td>claude-opus-4-6</td>")
		assert.Contains(t, body, "<th>claude-fable-5</th>")
		assert.Contains(t, body, "<th>Model changes</th>")
	})

	// empty-state
	t.Run("empty-state", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage?detail=models")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "No model changes.")
	})
}

func TestUsageDenialsDetail(t *testing.T) {
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

		response := get(server, "/fragments/sessions/s1/usage?detail=denials")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<th>Bash</th>")
		assert.Contains(t, body, "rm -rf /tmp/x")
	})

	// empty-state
	t.Run("empty-state", func(t *testing.T) {
		server, _ := newTestServer(t, "")

		response := get(server, "/fragments/sessions/s1/usage?detail=denials")
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "No permission denials.")
	})
}

func TestIsClaudeConfigPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/Users/k/.claude/plans/x.md", true},
		{"/repo/.claude/settings.json", true},
		{"/repo/.claude/worktrees/w/control/api.go", false},
		{"/repo/.claude/worktrees/w/.claude/settings.json", true},
		{`C:\Users\k\.claude\x.md`, true},
		{"/repo/control/api.go", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, isClaudeConfigPath(c.path), c.path)
	}
}
