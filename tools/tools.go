package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var errSessionSelectorMissing = errors.New("id or title parameter is required")

const (
	DefaultReturnedTurns = 20
)

func withMaxResultSize() *mcp.Meta {
	return mcp.NewMetaFromMap(map[string]any{
		"anthropic/maxResultSizeChars": 500_000,
	})
}

func Register(server *server.MCPServer, store *session.Store) {
	pageStore := &PageStore{
		PagesByRequestId: make(map[string]<-chan *sessionGetResult),
	}

	sessionGet := mcp.NewTool("session_get",
		mcp.WithDescription("Returns session data (turns, plan, git diff, uncommitted diff) for a session. Defaults to the most recently active session when id and title are omitted. Select sections with the turns/plan/diff/uncommitted_diff flags. Responses are paginated: if has_more is true, call again with the returned request_id to get the next page."),
		mcp.WithString("id",
			mcp.Description("Session ID (omit for most recent session)"),
		),
		mcp.WithString("title",
			mcp.Description("Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to agent when provided. For Codex, titles come from Codex's session index (thread name)."),
		),
		mcp.WithString("agent",
			mcp.Description("Agent: \"claude\" or \"codex\". Required when id and title are omitted and more than one agent is enabled."),
		),
		mcp.WithNumber("n",
			mcp.Description("Number of turns to return (default 20). Only applies to the turns section."),
		),
		mcp.WithBoolean("turns",
			mcp.Description("Return the session turns (default true)."),
		),
		mcp.WithBoolean("plan",
			mcp.Description("Return the session plan (default true)."),
		),
		mcp.WithBoolean("diff",
			mcp.Description("Return the pre-computed merge-base git diff against the inferred base branch, reported as diff_target (default true)."),
		),
		mcp.WithBoolean("uncommitted_diff",
			mcp.Description("Return the live uncommitted git diff (`git diff HEAD`) in the session's own working tree (default false)."),
		),
		mcp.WithString("request_id",
			mcp.Description("Pagination request ID from a previous response. Pass this to get the next page."),
		),
	)
	sessionGet.Meta = withMaxResultSize()
	server.AddTool(sessionGet, sessionGetHandler(store, pageStore))

	sessionList :=
		mcp.NewTool("session_list",
			mcp.WithDescription("Lists all sessions. Returns session ID, agent, last activity timestamp, whether a plan or diff is available, the inferred diff base branch (diff_target), and session metadata (cwd, git branch, model, origin)."),
			mcp.WithString("agent",
				mcp.Description("Agent: \"claude\" or \"codex\". Lists all sessions when omitted."),
			),
		)
	sessionList.Meta = withMaxResultSize()
	server.AddTool(sessionList, sessionListHandler(store))
}

func sessionGetHandler(s *session.Store, pageStore *PageStore) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		if reqId, ok := args["request_id"].(string); ok && reqId != "" {
			next, ok := pageStore.next(reqId)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("request_id %q not found or expired", reqId)), nil
			}

			if !pageStore.hasNext(reqId) {
				pageStore.remove(reqId)
				reqId = ""
			}

			result := &sessionGetResultPage{
				sessionGetResult: next,
				RequestId:        reqId,
				HasMore:          pageStore.hasNext(reqId),
			}
			return respond(ctx, result)
		}

		sess, err := resolveSession(s, request)
		if err != nil {
			if !errors.Is(err, errSessionSelectorMissing) {
				return mcp.NewToolResultError(err.Error()), nil
			}
			agent, agentErr := resolveAgentFromRequest(s, request)
			if agentErr != nil {
				return mcp.NewToolResultError(agentErr.Error()), nil
			}
			found, ok := s.Last(agent)
			if !ok {
				return mcp.NewToolResultText("no sessions found"), nil
			}
			sess = found
		}

		var turns, plan, diff, uncommitted string
		if boolArgFromRequest(request, "turns", true) {
			n := intArgFromRequest(request, "n")
			if n <= 0 {
				n = DefaultReturnedTurns
			}
			data, err := json.Marshal(sess.Turns(n))
			if err != nil {
				return nil, fmt.Errorf("marshaling turns: %w", err)
			}
			turns = string(data)
		}
		withDiff := boolArgFromRequest(request, "diff", true)
		if boolArgFromRequest(request, "plan", true) {
			plan = sess.PlanContent
		}
		if withDiff {
			diff = sess.DiffOutput
		}
		if boolArgFromRequest(request, "uncommitted_diff", false) {
			uncommitted = sess.UncommittedDiff
		}

		firstPage, nextPages := NewPageBuilder(maxResponseBytes(ctx)).build(turns, plan, diff, uncommitted)
		if withDiff {
			firstPage.DiffTarget = sess.DiffTarget
		}

		resultPage := newSessionGetResultPage(firstPage)
		if len(nextPages) == 0 {
			return respond(ctx, resultPage)
		}

		requestId := uuid.NewString()
		pageStore.add(requestId, nextPages)

		resultPage.WithRequestId(requestId)
		return respond(ctx, resultPage)
	}
}

func sessionListHandler(s *session.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agent, err := resolveAgentFromRequest(s, request)

		var sessions []*session.Session
		if err != nil {
			sessions = s.List()
		} else {
			sessions = s.List(agent)
		}
		items := make([]sessionListItem, len(sessions))
		for i, sess := range sessions {
			items[i] = sessionListItem{
				Id:          sess.Meta.SessionId,
				Agent:       sess.Agent,
				Title:       sess.Title,
				TitleSource: sess.TitleSource,
				LastActive:  sess.LastActive,
				HasPlan:     sess.PlanContent != "" || sess.PlanFilePath != "",
				HasDiff:     sess.DiffOutput != "",
				DiffTarget:  sess.DiffTarget,
				Meta:        sess.Meta,
			}
		}

		return respondWithStructured(map[string]any{"sessions": items})
	}
}

// resolveSession looks up a session by id or title from request args.
// Precedence: id > title.
func resolveSession(s *session.Store, request mcp.CallToolRequest) (*session.Session, error) {
	args := request.GetArguments()

	if id, ok := args["id"].(string); ok && id != "" {
		sess, found := s.GetById(session.Id(id))
		if !found {
			return nil, fmt.Errorf("session %q not found", id)
		}
		return sess, nil
	}

	if title, ok := args["title"].(string); ok && title != "" {
		agent, err := resolveAgentFilter(s, request)
		if err != nil {
			return nil, err
		}
		return s.GetByTitle(title, agent)
	}

	return nil, errSessionSelectorMissing
}

func resolveAgentFilter(s *session.Store, request mcp.CallToolRequest) (session.Agent, error) {
	raw, _ := request.GetArguments()["agent"].(string)
	if raw == "" {
		return "", nil
	}

	return s.ResolveAgent(session.Agent(raw))
}

func resolveAgentFromRequest(s *session.Store, request mcp.CallToolRequest) (session.Agent, error) {
	args := request.GetArguments()
	raw, _ := args["agent"].(string)

	return s.ResolveAgent(session.Agent(raw))
}
