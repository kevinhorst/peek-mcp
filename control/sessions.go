package control

import (
	"net/http"
	"slices"
	"time"

	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/tools"
)

const (
	pageSessions      = "sessions"
	tmplSessionsIndex = "sessions_index.html"
	tmplSessionDetail = "session_detail.html"
	tmplStats         = "stats.html"
	tmplStatsFragment = "_stats.html"
	tmplSessionList   = "_session_list.html"
	tmplTurns         = "_turns.html"
	tmplPlan          = "_plan.html"
	tmplDiff          = "_diff.html"
	tmplUsage         = "_usage.html"
	tmplEvents        = "_events.html"
	tmplMemory        = "_memory.html"
)

const maxEventsFragment = 100

type indexPage struct {
	Page     string
	Title    string
	BackLink string
}

type detailPage struct {
	Page     string
	Title    string
	Summary  sessionSummary
	BackLink string
}

type sessionListData struct {
	Agent      session.Agent
	Sessions   []sessionSummary
	LastActive time.Time
	Total      int
	Offset     int
	PrevOffset int
	NextOffset int
	HasPrev    bool
	HasNext    bool
	RangeEnd   int
}

type turnsData struct {
	Id       session.Id
	Turns    []*session.Turn
	Subagent string
	Tabs     []subagentTab
}

type subagentTab struct {
	Id          string
	Label       string
	Description string
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
	s.renderFragment(w, tmplSessionsIndex, indexPage{Page: pageSessions, Title: "Peek", BackLink: s.config.BackLink})
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
	s.renderFragment(w, tmplSessionDetail, detailPage{Page: pageSessions, Title: title, Summary: summary, BackLink: s.config.BackLink})
}

func (s *Server) handleSessionsFragment(w http.ResponseWriter, r *http.Request) {
	agents, ok := agentParam(r)
	if !ok || agents == nil {
		respondBadRequest("agent must be \"claude\" or \"codex\"", w)
		return
	}
	offset, ok := intParam(r, "offset", 0)
	if !ok {
		respondBadRequest("offset must be a non-negative integer", w)
		return
	}
	data := sessionListData{Agent: agents[0], Sessions: make([]sessionSummary, 0), Offset: offset}
	s.store.WithSessions(agents, func(sessions []*session.Session) {
		data.Total = len(sessions)
		if len(sessions) > 0 {
			data.LastActive = sessions[0].LastActive
		}
		for _, sess := range pageSlice(sessions, offset, defaultSessionLimit) {
			data.Sessions = append(data.Sessions, newSessionSummary(sess))
		}
	})
	data.HasPrev = offset > 0
	data.PrevOffset = max(0, offset-defaultSessionLimit)
	data.NextOffset = offset + defaultSessionLimit
	data.HasNext = data.NextOffset < data.Total
	data.RangeEnd = offset + len(data.Sessions)
	s.renderFragment(w, tmplSessionList, data)
}

type usageData struct {
	Id           session.Id
	Counters     session.Counters
	Usage        session.Usage
	TotalTokens  int
	CachePercent string
	PlanVersions int
	SessionTime  string
	IdleTime     string
	ActiveTime   string
	Detail       string
	Cost         *costData
	Denials      *denialsData
	Models       *modelsData
	Plans        *planVersionsData
	Skills       *skillsData
	Subagents    *subagentsData
	TouchedFiles int
	Files        *filesData
}

type eventsData struct {
	Id     session.Id
	Events []*tools.EventEntry
}

func (s *Server) handleUsageFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	data := usageData{Id: id, Detail: usageDetailParam(r)}
	if !s.store.WithSession(id, func(sess *session.Session) {
		data.Counters = sess.Counters
		data.Usage = *sess.CurrentUsage()
		data.TotalTokens = displayTotalTokens(&data.Usage)
		data.CachePercent = cachePercent(sess.Agent, &data.Usage)
		data.PlanVersions = len(sess.PlanRevisions)
		data.TouchedFiles = len(sess.TouchedFiles)
		if !sess.StartedAt.IsZero() {
			wall := sess.LastActive.Sub(sess.StartedAt)
			data.SessionTime = wall.Round(time.Second).String()
			data.IdleTime = sess.Idle.Round(time.Second).String()
			data.ActiveTime = (wall - sess.Idle).Round(time.Second).String()
		}
		switch data.Detail {
		case usageDetailCost:
			cost := newCostData(id, sess.Agent, sess.Meta.Model, sess.CurrentUsage())
			data.Cost = &cost
		case usageDetailDenials:
			data.Denials = newDenialsData(sess)
		case usageDetailModels:
			data.Models = newModelsData(sess)
		case usageDetailPlans:
			data.Plans = newPlanVersionsData(sess)
		case usageDetailSkills:
			data.Skills = newSkillsData(id, sess)
		case usageDetailSubagents:
			data.Subagents = newSubagentsData(id, sess)
		case usageDetailFiles:
			data.Files = newFilesData(sess)
		}
	}) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplUsage, data)
}

func (s *Server) handleEventsFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	data := eventsData{Id: id}
	if !s.store.WithSession(id, func(sess *session.Session) {
		all := sess.Events.All()
		slices.Reverse(all)
		if len(all) > maxEventsFragment {
			all = all[:maxEventsFragment]
		}
		data.Events = tools.NewEventEntries(all)
	}) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplEvents, data)
}

func (s *Server) handleTurnsFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	data := turnsData{Id: id, Subagent: r.URL.Query().Get("subagent")}
	if !s.store.WithSession(id, func(sess *session.Session) {
		for _, agentId := range sess.SubagentIds() {
			stat := sess.Subagents[agentId]
			data.Tabs = append(data.Tabs, subagentTab{Id: agentId, Label: subagentTabLabel(agentId, stat), Description: stat.Description})
		}
		if data.Subagent != "" {
			if turns, ok := sess.SubagentTurns(data.Subagent, tools.DefaultReturnedTurns); ok {
				data.Turns = turns
			}
			return
		}
		data.Turns = sess.Turns(tools.DefaultReturnedTurns)
	}) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplTurns, data)
}

func subagentTabLabel(agentId string, stat *session.SubagentStat) string {
	if stat.AgentType != "" {
		return stat.AgentType
	}

	runes := []rune(agentId)
	if len(runes) > 8 {
		return string(runes[:8])
	}
	return agentId
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

type memoryFact struct {
	Name string
	Type string
	Body string
}

type memoryData struct {
	Id          session.Id
	IndexHTML   any
	Facts       []memoryFact
	Truncated   bool
	Unavailable string
}

func (s *Server) handleMemoryFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	var agent session.Agent
	var transcriptPath string
	if !s.store.WithSession(id, func(sess *session.Session) {
		agent = sess.Agent
		transcriptPath = sess.FilePath
	}) {
		respondNotFound("unknown session", w)
		return
	}

	data := memoryData{Id: id}
	switch {
	case agent != session.AgentClaude:
		data.Unavailable = "memory is not available for codex sessions"
	case transcriptPath == "":
		data.Unavailable = "transcript path unknown"
	default:
		memory, err := claude.ReadMemory(transcriptPath)
		if err != nil {
			data.Unavailable = err.Error()
			break
		}
		data.Truncated = memory.IsTruncated
		if memory.Index != "" {
			html, err := renderMarkdown([]byte(memory.Index))
			if err != nil {
				respondInternalServerError(err, w)
				return
			}
			data.IndexHTML = html
		}
		for _, fact := range memory.Facts {
			data.Facts = append(data.Facts, memoryFact{Name: fact.Name, Type: fact.Type, Body: fact.Body})
		}
	}
	s.renderFragment(w, tmplMemory, data)
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
