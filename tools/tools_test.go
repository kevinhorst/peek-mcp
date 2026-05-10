package tools

import (
	"context"
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

func addSession(s *session.Store, id session.Id) {
	s.AddTurnBySessionId(id, session.SourceClaude, &session.Turn{
		Role:      session.RoleUser,
		Text:      "hello",
		Timestamp: time.Now(),
		Meta:      &session.Meta{SessionId: id},
	})
}
