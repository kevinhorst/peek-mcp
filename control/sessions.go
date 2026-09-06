package control

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
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
	Id           session.Id
	Turns        []*session.Turn
	Subagent     string
	Groups       []subagentGroup
	HasSubagents bool
	Info         *turnsInfo
	Role         string
	RoleQueries  map[string]string
	ShowThinking bool
	HasThinking  bool
	Query        string
	MainQuery    string
	ToggleQuery  string
}

type subagentGroup struct {
	Name string
	Open bool
	Rows []subagentTab
}

type subagentTab struct {
	Id          string
	Label       string
	Description string
	LastActive  time.Time
	Active      bool
	Query       string
}

type turnsInfo struct {
	Id          string
	Description string
	Model       string
	StartedAt   time.Time
	LastActive  time.Time
	Duration    string
	Usage       session.Usage
	Tokens      int
	Cost        string
}

func newMainInfo(sess *session.Session) *turnsInfo {
	usage := sess.CurrentUsage()
	cost := newCostData(sess.Meta.SessionId, sess.Agent, sess.Meta.Model, usage)
	info := &turnsInfo{
		Id:          string(sess.Meta.SessionId),
		Description: sess.Title,
		Model:       sess.Meta.Model,
		Usage:       *usage,
		Tokens:      displayTotalTokens(usage),
		Cost:        cost.Total,
	}
	if !sess.StartedAt.IsZero() {
		info.StartedAt = sess.StartedAt
		info.LastActive = sess.LastActive
		info.Duration = sess.LastActive.Sub(sess.StartedAt).Round(time.Second).String()
	}
	return info
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
	StartedAt    time.Time
	LastActive   time.Time
	Usage        session.Usage
	TotalTokens  int
	CachePercent string
	PlanVersions int
	SessionTime  string
	IdleTime     string
	ActiveTime   string
	Detail       string
	Sort         sortState
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
	key, dir := usageSortParam(r, data.Detail)
	data.Sort = sortState{Id: id, Detail: data.Detail, Key: key, Dir: dir, Cols: usageColsParam(r)}
	if !s.store.WithSession(id, func(sess *session.Session) {
		data.Counters = sess.Counters
		data.Usage = aggregateUsage(sess)
		data.TotalTokens = displayTotalTokens(&data.Usage)
		data.CachePercent = cachePercent(sess.Agent, &data.Usage)
		data.PlanVersions = len(sess.PlanRevisions)
		data.TouchedFiles = len(sess.TouchedFiles)
		if !sess.StartedAt.IsZero() {
			data.StartedAt = sess.StartedAt
			data.LastActive = sess.LastActive
			wall := sess.LastActive.Sub(sess.StartedAt)
			data.SessionTime = wall.Round(time.Second).String()
			data.IdleTime = sess.Idle.Round(time.Second).String()
			data.ActiveTime = (wall - sess.Idle).Round(time.Second).String()
		}
		switch data.Detail {
		case usageDetailCost:
			cost := newSessionCostData(id, sess)
			data.Cost = &cost
		case usageDetailDenials:
			data.Denials = newDenialsData(sess)
		case usageDetailModels:
			data.Models = newModelsData(sess)
		case usageDetailPlans:
			data.Plans = newPlanVersionsData(sess)
		case usageDetailSkills:
			data.Skills = newSkillsData(id, sess, data.Sort)
		case usageDetailSubagents:
			data.Subagents = newSubagentsData(id, sess, data.Sort)
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
	data := turnsData{
		Id:           id,
		Subagent:     r.URL.Query().Get("subagent"),
		Role:         roleParam(r),
		ShowThinking: r.URL.Query().Get("thinking") != "off",
	}
	if !s.store.WithSession(id, func(sess *session.Session) {
		data.Groups = newSubagentGroups(sess, data.Subagent, data.Role, data.ShowThinking)
		data.HasSubagents = len(data.Groups) > 0
		if data.Subagent != "" {
			if turns, ok := sess.SubagentTurns(data.Subagent, session.AllTurns); ok {
				data.Turns = turns
				data.Info = newSubagentInfo(id, data.Subagent, sess)
			}
			return
		}
		data.Turns = sess.Turns(session.AllTurns)
		data.Info = newMainInfo(sess)
	}) {
		respondNotFound("unknown session", w)
		return
	}
	data.Turns = slices.Clone(data.Turns)
	slices.Reverse(data.Turns)
	for _, turn := range data.Turns {
		if turn.Thinking != "" {
			data.HasThinking = true
			break
		}
	}
	if data.Role != "" {
		filtered := data.Turns[:0]
		for _, turn := range data.Turns {
			if string(turn.Role) == data.Role {
				filtered = append(filtered, turn)
			}
		}
		data.Turns = filtered
	}
	data.Query = turnsQuery(data.Subagent, data.Role, data.ShowThinking)
	data.MainQuery = turnsQuery("", data.Role, data.ShowThinking)
	data.ToggleQuery = turnsQuery(data.Subagent, data.Role, !data.ShowThinking)
	data.RoleQueries = map[string]string{
		"":          turnsQuery(data.Subagent, "", data.ShowThinking),
		"user":      turnsQuery(data.Subagent, "user", data.ShowThinking),
		"assistant": turnsQuery(data.Subagent, "assistant", data.ShowThinking),
	}
	s.renderFragment(w, tmplTurns, data)
}

func roleParam(r *http.Request) string {
	switch role := r.URL.Query().Get("role"); role {
	case "user", "assistant":
		return role
	}
	return ""
}

func newSubagentGroups(sess *session.Session, selected, role string, showThinking bool) []subagentGroup {
	byType := make(map[string]int)
	var groups []subagentGroup
	for _, agentId := range sess.SubagentIds() {
		stat := sess.Subagents[agentId]
		name := stat.AgentType
		if name == "" {
			name = "unknown"
		}
		index, ok := byType[name]
		if !ok {
			index = len(groups)
			byType[name] = index
			groups = append(groups, subagentGroup{Name: name})
		}
		groups[index].Rows = append(groups[index].Rows, subagentTab{
			Id:          agentId,
			Label:       subagentTabLabel(agentId, stat),
			Description: stat.Description,
			LastActive:  stat.LastActive,
			Active:      agentId == selected,
			Query:       turnsQuery(agentId, role, showThinking),
		})
		if agentId == selected {
			groups[index].Open = true
		}
	}
	slices.SortFunc(groups, func(a, b subagentGroup) int { return strings.Compare(a.Name, b.Name) })
	return groups
}

func newSubagentInfo(id session.Id, agentId string, sess *session.Session) *turnsInfo {
	stat, ok := sess.Subagents[agentId]
	if !ok {
		return nil
	}

	model := subagentModel(stat, sess)
	cost := newCostData(id, sess.Agent, model, &stat.Usage)
	return &turnsInfo{
		Id:          agentId,
		Description: stat.Description,
		Model:       model,
		StartedAt:   stat.FirstActive,
		LastActive:  stat.LastActive,
		Duration:    stat.LastActive.Sub(stat.FirstActive).Round(time.Second).String(),
		Usage:       stat.Usage,
		Tokens:      displayTotalTokens(&stat.Usage),
		Cost:        cost.Total,
	}
}

func turnsQuery(subagent, role string, showThinking bool) string {
	values := url.Values{}
	if subagent != "" {
		values.Set("subagent", subagent)
	}
	if role != "" {
		values.Set("role", role)
	}
	if !showThinking {
		values.Set("thinking", "off")
	}
	if len(values) == 0 {
		return ""
	}
	return "?" + values.Encode()
}

func subagentTabLabel(agentId string, stat *session.SubagentStat) string {
	runes := []rune(agentId)
	if stat.AgentType != "" {
		suffix := agentId
		if len(runes) > 4 {
			suffix = string(runes[len(runes)-4:])
		}
		return stat.AgentType + " " + suffix
	}

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
	s.serveDiffFragment(w, r, "diff", func(id session.Id) (string, string, bool) {
		var target string
		found := s.store.WithSession(id, func(sess *session.Session) {
			target = sess.DiffTarget
		})
		if !found {
			return "", "", false
		}

		content, loadedTarget, ok := s.store.LoadDiff(id)
		if ok {
			target = loadedTarget
		}
		return content, target, true
	})
}

func (s *Server) handleUncommittedDiffFragment(w http.ResponseWriter, r *http.Request) {
	s.serveDiffFragment(w, r, "uncommitted-diff", func(id session.Id) (string, string, bool) {
		var diff string
		found := s.store.WithSession(id, func(sess *session.Session) { diff = sess.UncommittedDiff })
		return diff, "", found
	})
}

func (s *Server) serveDiffFragment(w http.ResponseWriter, r *http.Request, kind string, load func(session.Id) (string, string, bool)) {
	id := session.Id(r.PathValue("id"))
	diff, target, found := load(id)
	if !found {
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
