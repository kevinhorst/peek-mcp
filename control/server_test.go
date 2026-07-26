package control

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore() (*session.Store, *events.Broker) {
	broker := events.NewBroker()
	store := session.NewStore(10, broker, session.AgentClaude, session.AgentCodex)
	now := time.Now()

	store.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		CustomTitle: "Login simplification",
		Meta:        &session.Meta{SessionId: "s1"},
	})
	store.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		Role:      session.RoleUser,
		Text:      "What does this do?",
		Timestamp: now.Add(-time.Hour),
		RequestId: "r1",
		Usage:     &session.Usage{InputTokens: 10, OutputTokens: 5},
		Meta:      &session.Meta{SessionId: "s1", CWD: "/project", GitBranch: "main", Model: "opus"},
	})
	store.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		Role:      session.RoleAssistant,
		Text:      "It does things.",
		Timestamp: now.Add(-59 * time.Minute),
		RequestId: "r2",
		Meta:      &session.Meta{SessionId: "s1"},
	})
	store.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		PlanFilePath: "/plans/p.md",
		PlanContent:  "# Plan\n\ncontent",
		Meta:         &session.Meta{SessionId: "s1"},
	})
	store.UpdateDiff("s1", "main", "diff-content")
	store.UpdateUncommittedDiff("s1", "uncommitted-content")

	store.AddTurnBySessionId("s2", session.AgentCodex, &session.Turn{
		Role:      session.RoleUser,
		Text:      "Refactor auth",
		Timestamp: now,
		Meta:      &session.Meta{SessionId: "s2", CWD: "/project"},
	})

	return store, broker
}

func newTestServer(t *testing.T, token string) (*Server, *events.Broker) {
	store, broker := newTestStore()
	server, err := New(&Options{Store: store, Broker: broker, Token: token, Version: "test", Depth: 10})
	require.NoError(t, err)
	return server, broker
}

func get(server *Server, path string, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4243"+path, nil)
	for _, fn := range mutate {
		fn(request)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestCheckHost(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		host string
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-localhost-with-port",
		_shouldPass: true,

		host: "localhost:4243",
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "pass-loopback-without-port",
		_shouldPass: true,

		host: "127.0.0.1",
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "pass-ipv6-loopback",
		_shouldPass: true,

		host: "[::1]:4243",
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-rebinding-host",
		_shouldPass: false,

		host: "evil.com",
	}
	tests = append(tests, test)

	server, _ := newTestServer(t, "")
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			response := get(server, "/api/healthz", func(r *http.Request) { r.Host = test.host })
			if test._shouldPass {
				assert.Equal(t, http.StatusOK, response.Code)
			} else {
				assert.Equal(t, http.StatusForbidden, response.Code)
			}
		})
	}
}

func TestAuth(t *testing.T) {
	type testCase struct {
		_id           string
		_expectedCode int

		token  string
		mutate func(*http.Request)
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:           "pass-no-token-configured",
		_expectedCode: http.StatusOK,

		token:  "",
		mutate: func(r *http.Request) {},
	}
	tests = append(tests, test)

	test = &testCase{
		_id:           "pass-bearer",
		_expectedCode: http.StatusOK,

		token:  "secret123",
		mutate: func(r *http.Request) { r.Header.Set("Authorization", "Bearer secret123") },
	}
	tests = append(tests, test)

	test = &testCase{
		_id:           "fail-wrong-bearer",
		_expectedCode: http.StatusUnauthorized,

		token:  "secret123",
		mutate: func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") },
	}
	tests = append(tests, test)

	test = &testCase{
		_id:           "fail-missing-token",
		_expectedCode: http.StatusUnauthorized,

		token:  "secret123",
		mutate: func(r *http.Request) {},
	}
	tests = append(tests, test)

	test = &testCase{
		_id:           "pass-cookie",
		_expectedCode: http.StatusOK,

		token:  "secret123",
		mutate: func(r *http.Request) { r.AddCookie(&http.Cookie{Name: tokenCookie, Value: "secret123"}) },
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			server, _ := newTestServer(t, test.token)
			response := get(server, "/api/sessions", func(r *http.Request) { test.mutate(r) })
			assert.Equal(t, test._expectedCode, response.Code)
		})
	}
}

func TestAuth_QueryTokenSetsCookie(t *testing.T) {
	server, _ := newTestServer(t, "secret123")

	response := get(server, "/?token=secret123")
	assert.Equal(t, http.StatusOK, response.Code)

	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, tokenCookie, cookies[0].Name)
	assert.Equal(t, "secret123", cookies[0].Value)
	assert.True(t, cookies[0].HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)
}

func TestAuth_HealthzExempt(t *testing.T) {
	server, _ := newTestServer(t, "secret123")

	response := get(server, "/api/healthz")
	assert.Equal(t, http.StatusOK, response.Code)
}
