package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func Register(srv *server.MCPServer, store *session.Store) {
	srv.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"peek://session/{id}",
			"Session Full",
			mcp.WithTemplateDescription("Returns the full session data (turns, plan, and git diff) with no truncation. Use this to access large session data that may be truncated when accessed via tools."),
			mcp.WithTemplateMIMEType("application/json"),
		),
		sessionFullResourceHandler(store),
	)

	srv.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"peek://sessions",
			"Session List",
			mcp.WithTemplateDescription("Lists all active sessions with metadata (ID, last active time, whether a plan or diff is available)."),
			mcp.WithTemplateMIMEType("application/json"),
		),
		sessionListResourceHandler(store),
	)
}

type sessionResourceResult struct {
	Turns []*session.Turn `json:"turns"`
	Plan  string          `json:"plan,omitempty"`
	Diff  string          `json:"diff,omitempty"`
}

func sessionFullResourceHandler(store *session.Store) server.ResourceTemplateHandlerFunc {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		id, _ := request.Params.Arguments["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("missing session id")
		}

		sess, ok := store.GetById(session.Id(id))
		if !ok {
			return nil, fmt.Errorf("session %q not found", id)
		}

		result := sessionResourceResult{
			Turns: sess.Turns(1<<31 - 1), // all turns
			Plan:  sess.PlanContent,
			Diff:  sess.DiffOutput,
		}

		data, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("marshaling session: %w", err)
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}

type sessionListEntry struct {
	Id         session.Id `json:"id"`
	LastActive string     `json:"last_active"`
	HasPlan    bool       `json:"has_plan"`
	HasDiff    bool       `json:"has_diff"`
}

func sessionListResourceHandler(store *session.Store) server.ResourceTemplateHandlerFunc {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		sessions := store.List()
		entries := make([]sessionListEntry, len(sessions))
		for i, sess := range sessions {
			entries[i] = sessionListEntry{
				Id:         sess.Meta.SessionId,
				LastActive: sess.LastActive.Format("2006-01-02T15:04:05Z"),
				HasPlan:    sess.PlanContent != "",
				HasDiff:    sess.DiffOutput != "",
			}
		}

		data, err := json.Marshal(map[string]any{"sessions": entries})
		if err != nil {
			return nil, fmt.Errorf("marshaling session list: %w", err)
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}
