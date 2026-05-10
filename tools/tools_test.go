package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func TestSessionPlanHandler(t *testing.T) {
	type testCase struct {
		_id          string
		_wantIsError bool
		_wantText    string

		setup func(*session.Store)
		args  map[string]any
	}

	tests := make([]*testCase, 0)

	// no-sessions
	tests = append(tests, &testCase{
		_id:       "no-sessions",
		_wantText: "session_plan: No sessions found",
		setup:     func(*session.Store) {},
		args:      nil,
	})

	// most-recent-no-plan
	tests = append(tests, &testCase{
		_id:       "most-recent-no-plan",
		_wantText: "No plan found for this session",
		setup: func(s *session.Store) {
			addSession(s, "sess1")
		},
		args: nil,
	})

	// most-recent-with-plan
	tests = append(tests, &testCase{
		_id:       "most-recent-with-plan",
		_wantText: "# My Plan",
		setup: func(s *session.Store) {
			addSession(s, "sess1")
			sess, _ := s.GetById("sess1")
			sess.PlanContent = "# My Plan"
		},
		args: nil,
	})

	// by-id-found-with-plan
	tests = append(tests, &testCase{
		_id:       "by-id-found-with-plan",
		_wantText: "# Plan B",
		setup: func(s *session.Store) {
			addSession(s, "sess1")
			addSession(s, "sess2")
			sess, _ := s.GetById("sess2")
			sess.PlanContent = "# Plan B"
		},
		args: map[string]any{"id": "sess2"},
	})

	// by-id-found-no-plan
	tests = append(tests, &testCase{
		_id:       "by-id-found-no-plan",
		_wantText: "No plan found for this session",
		setup: func(s *session.Store) {
			addSession(s, "sess1")
		},
		args: map[string]any{"id": "sess1"},
	})

	// by-id-not-found
	tests = append(tests, &testCase{
		_id:          "by-id-not-found",
		_wantIsError: true,
		_wantText:    `session "missing" not found`,
		setup:        func(*session.Store) {},
		args:         map[string]any{"id": "missing"},
	})

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			store := session.NewStore(10)
			test.setup(store)

			handler := sessionPlanHandler(store)
			req := mcp.CallToolRequest{}
			req.Params.Arguments = test.args

			result, err := handler(context.Background(), req)
			assert.NoError(t, err)
			assert.Equal(t, test._wantIsError, result.IsError)
			assert.Equal(t, test._wantText, result.Content[0].(mcp.TextContent).Text)
		})
	}
}

func TestSessionListHandler(t *testing.T) {
	type testCase struct {
		_id      string
		setup    func(*session.Store)
		validate func(*testing.T, string)
	}

	tests := []*testCase{
		{
			_id:   "no-sessions",
			setup: func(*session.Store) {},
			validate: func(t *testing.T, text string) {
				assert.Equal(t, "[]", text)
			},
		},
		{
			_id: "one-session-no-plan-no-diff",
			setup: func(s *session.Store) {
				addSession(s, "sess1")
			},
			validate: func(t *testing.T, text string) {
				var items []sessionListItem
				assert.NoError(t, json.Unmarshal([]byte(text), &items))
				assert.Len(t, items, 1)
				assert.Equal(t, session.Id("sess1"), items[0].Id)
				assert.False(t, items[0].HasPlan)
				assert.False(t, items[0].HasDiff)
				assert.False(t, items[0].LastActive.IsZero())
			},
		},
		{
			_id: "one-session-with-plan",
			setup: func(s *session.Store) {
				addSession(s, "sess1")
				sess, _ := s.GetById("sess1")
				sess.PlanContent = "# My Plan"
			},
			validate: func(t *testing.T, text string) {
				var items []sessionListItem
				assert.NoError(t, json.Unmarshal([]byte(text), &items))
				assert.True(t, items[0].HasPlan)
				assert.False(t, items[0].HasDiff)
			},
		},
		{
			_id: "one-session-with-diff",
			setup: func(s *session.Store) {
				addSession(s, "sess1")
				sess, _ := s.GetById("sess1")
				sess.DiffOutput = "diff --git a/foo.go b/foo.go"
			},
			validate: func(t *testing.T, text string) {
				var items []sessionListItem
				assert.NoError(t, json.Unmarshal([]byte(text), &items))
				assert.False(t, items[0].HasPlan)
				assert.True(t, items[0].HasDiff)
			},
		},
		{
			_id: "two-sessions-sorted-by-last-active",
			setup: func(s *session.Store) {
				addSessionAt(s, "older", time.Now().Add(-time.Hour))
				addSessionAt(s, "newer", time.Now())
			},
			validate: func(t *testing.T, text string) {
				var items []sessionListItem
				assert.NoError(t, json.Unmarshal([]byte(text), &items))
				assert.Len(t, items, 2)
				assert.Equal(t, session.Id("newer"), items[0].Id)
				assert.Equal(t, session.Id("older"), items[1].Id)
			},
		},
	}

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			store := session.NewStore(10)
			test.setup(store)

			handler := sessionListHandler(store)
			req := mcp.CallToolRequest{}

			result, err := handler(context.Background(), req)
			assert.NoError(t, err)
			assert.False(t, result.IsError)
			test.validate(t, result.Content[0].(mcp.TextContent).Text)
		})
	}
}

func addSession(s *session.Store, id session.Id) {
	addSessionAt(s, id, time.Now())
}

func addSessionAt(s *session.Store, id session.Id, ts time.Time) {
	s.AddTurnBySessionId(id, session.SourceClaude, &session.Turn{
		Role:      session.RoleUser,
		Text:      "hello",
		Timestamp: ts,
		Meta:      &session.Meta{SessionId: id},
	})
}
