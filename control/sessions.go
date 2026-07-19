package control

import (
	"net/http"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/tools"
)

const (
	pageSessions      = "sessions"
	tmplSessionsIndex = "sessions_index.html"
	tmplSessionDetail = "session_detail.html"
	tmplSessionList   = "_session_list.html"
	tmplTurns         = "_turns.html"
	tmplPlan          = "_plan.html"
	tmplDiff          = "_diff.html"
)

type indexPage struct {
	Page  string
	Title string
}

type detailPage struct {
	Page    string
	Title   string
	Summary sessionSummary
}

type sessionListData struct {
	Sessions []sessionSummary
}

type turnsData struct {
	Id    session.Id
	Turns []*session.Turn
}

type planData struct {
	Id       session.Id
	PlanHTML any
	Empty    bool
}

type diffData struct {
	Id        session.Id
	Kind      string
	Target    string
	Diff      string
	Truncated bool
	Empty     bool
}

func (s *Server) handleSessionsPage(w http.ResponseWriter, r *http.Request) {
	s.renderFragment(w, tmplSessionsIndex, indexPage{Page: pageSessions, Title: "Peek"})
}

func (s *Server) handleSessionDetailPage(w http.ResponseWriter, r *http.Request) {
	var summary sessionSummary
	found := s.store.WithSession(session.Id(r.PathValue("id")), func(sess *session.Session) {
		summary = newSessionSummary(sess)
	})
	if !found {
		respondNotFound("unknown session", w)
		return
	}
	title := summary.Title
	if title == "" {
		title = string(summary.Id)
	}
	s.renderFragment(w, tmplSessionDetail, detailPage{Page: pageSessions, Title: title, Summary: summary})
}

func (s *Server) handleSessionsFragment(w http.ResponseWriter, r *http.Request) {
	data := sessionListData{Sessions: make([]sessionSummary, 0)}
	s.store.WithSessions(nil, func(sessions []*session.Session) {
		for _, sess := range sessions {
			data.Sessions = append(data.Sessions, newSessionSummary(sess))
			if len(data.Sessions) == defaultSessionLimit {
				break
			}
		}
	})
	s.renderFragment(w, tmplSessionList, data)
}

func (s *Server) handleTurnsFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	data := turnsData{Id: id}
	if !s.store.WithSession(id, func(sess *session.Session) { data.Turns = sess.Turns(defaultTurns) }) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplTurns, data)
}

func (s *Server) handlePlanFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	var content string
	if !s.store.WithSession(id, func(sess *session.Session) { content = sess.PlanContent }) {
		respondNotFound("unknown session", w)
		return
	}
	data := planData{Id: id, Empty: content == ""}
	if content != "" {
		html, err := renderMarkdown([]byte(content))
		if err != nil {
			respondInternalServerError(err, w)
			return
		}
		data.PlanHTML = html
	}
	s.renderFragment(w, tmplPlan, data)
}

func (s *Server) handleDiffFragment(w http.ResponseWriter, r *http.Request) {
	s.serveDiffFragment(w, r, "diff", func(sess *session.Session) (string, string) {
		return sess.DiffOutput, sess.DiffTarget
	})
}

func (s *Server) handleUncommittedDiffFragment(w http.ResponseWriter, r *http.Request) {
	s.serveDiffFragment(w, r, "uncommitted-diff", func(sess *session.Session) (string, string) {
		return sess.UncommittedDiff, ""
	})
}

func (s *Server) serveDiffFragment(w http.ResponseWriter, r *http.Request, kind string, extract func(*session.Session) (string, string)) {
	id := session.Id(r.PathValue("id"))
	var diff, target string
	if !s.store.WithSession(id, func(sess *session.Session) { diff, target = extract(sess) }) {
		respondNotFound("unknown session", w)
		return
	}
	data := diffData{Id: id, Kind: kind, Target: target, Empty: diff == ""}
	if len(diff) > defaultDiffSize {
		diff = tools.UTF8SafeSlice(diff, defaultDiffSize)
		data.Truncated = true
	}
	data.Diff = diff
	s.renderFragment(w, tmplDiff, data)
}
