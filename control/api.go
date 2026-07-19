package control

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/tools"
)

const (
	defaultSessionLimit = 50
	maxSessionLimit     = 200
	defaultDiffSize     = 256 * 1024
	defaultTurns        = 5
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("control: encode failed", "err", err)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, healthzResponse{Status: "ok", Version: s.version})
}

func intParam(r *http.Request, name string, fallback int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	var agents []session.Agent
	switch agent := r.URL.Query().Get("agent"); agent {
	case "":
	case string(session.AgentClaude), string(session.AgentCodex):
		agents = []session.Agent{session.Agent(agent)}
	default:
		respondBadRequest("agent must be \"claude\" or \"codex\"", w)
		return
	}
	limit, ok := intParam(r, "limit", defaultSessionLimit)
	if !ok {
		respondBadRequest("limit must be a non-negative integer", w)
		return
	}
	if limit == 0 || limit > maxSessionLimit {
		limit = maxSessionLimit
	}
	title := strings.ToLower(r.URL.Query().Get("title"))

	list := make([]sessionSummary, 0)
	s.store.WithSessions(agents, func(sessions []*session.Session) {
		for _, sess := range sessions {
			if title != "" && !strings.Contains(strings.ToLower(sess.Title), title) {
				continue
			}
			list = append(list, newSessionSummary(sess))
			if len(list) == limit {
				break
			}
		}
	})
	writeJSON(w, sessionsResponse{Sessions: list})
}

func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	var detail sessionDetail
	found := s.store.WithSession(session.Id(r.PathValue("id")), func(sess *session.Session) {
		detail = sessionDetail{
			sessionSummary: newSessionSummary(sess),
			TotalUsage:     sess.TotalUsage,
			DiffTarget:     sess.DiffTarget,
		}
	})
	if !found {
		respondNotFound("unknown session", w)
		return
	}
	writeJSON(w, detail)
}

func (s *Server) handleTurns(w http.ResponseWriter, r *http.Request) {
	n, ok := intParam(r, "n", defaultTurns)
	if !ok {
		respondBadRequest("n must be a non-negative integer", w)
		return
	}
	if n == 0 || n > s.depth {
		n = s.depth
	}

	var turns []*session.Turn
	found := s.store.WithSession(session.Id(r.PathValue("id")), func(sess *session.Session) {
		turns = sess.Turns(n)
	})
	if !found {
		respondNotFound("unknown session", w)
		return
	}
	writeJSON(w, turnsResponse{Turns: turns})
}

func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	var plan planResponse
	found := s.store.WithSession(session.Id(r.PathValue("id")), func(sess *session.Session) {
		plan = planResponse{PlanContent: sess.PlanContent, PlanFilePath: sess.PlanFilePath}
	})
	if !found {
		respondNotFound("unknown session", w)
		return
	}
	if plan.PlanContent == "" && plan.PlanFilePath == "" {
		respondNotFound("no plan for session", w)
		return
	}
	writeJSON(w, plan)
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	s.serveDiff(w, r, func(sess *session.Session) diffResponse {
		return diffResponse{Target: sess.DiffTarget, Diff: sess.DiffOutput}
	})
}

func (s *Server) handleUncommittedDiff(w http.ResponseWriter, r *http.Request) {
	s.serveDiff(w, r, func(sess *session.Session) diffResponse {
		return diffResponse{Diff: sess.UncommittedDiff}
	})
}

func (s *Server) serveDiff(w http.ResponseWriter, r *http.Request, extract func(*session.Session) diffResponse) {
	size, ok := intParam(r, "size", defaultDiffSize)
	if !ok {
		respondBadRequest("size must be a non-negative integer", w)
		return
	}

	var resp diffResponse
	found := s.store.WithSession(session.Id(r.PathValue("id")), func(sess *session.Session) {
		resp = extract(sess)
	})
	if !found {
		respondNotFound("unknown session", w)
		return
	}
	if size > 0 && len(resp.Diff) > size {
		resp.Diff = tools.UTF8SafeSlice(resp.Diff, size)
		resp.Truncated = true
	}
	writeJSON(w, resp)
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	var usage usageResponse
	found := s.store.WithSession(session.Id(r.PathValue("id")), func(sess *session.Session) {
		usage = usageResponse{TotalUsage: sess.TotalUsage}
	})
	if !found {
		respondNotFound("unknown session", w)
		return
	}
	writeJSON(w, usage)
}
