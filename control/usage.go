package control

import (
	"fmt"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kevinhorst/peek-mcp/pricing"
	"github.com/kevinhorst/peek-mcp/session"
)

const (
	sortDirAsc  = "asc"
	sortDirDesc = "desc"
)

var usageSortKeys = map[string][]string{
	usageDetailSkills:    {"agent", "skill", "started", "ended", "tokens"},
	usageDetailSubagents: {"agent", "model", "started", "lastactive", "tokens", "cost"},
}

func usageSortParam(r *http.Request, detail string) (string, string) {
	key := r.URL.Query().Get("sort")
	if !slices.Contains(usageSortKeys[detail], key) {
		return "", ""
	}
	if r.URL.Query().Get("dir") == sortDirDesc {
		return key, sortDirDesc
	}
	return key, sortDirAsc
}

type sortState struct {
	Id     session.Id
	Detail string
	Key    string
	Dir    string
}

func (s sortState) Query(column string) string {
	dir := sortDirAsc
	if s.Key == column && s.Dir == sortDirAsc {
		dir = sortDirDesc
	}
	return fmt.Sprintf("?detail=%s&sort=%s&dir=%s", s.Detail, column, dir)
}

func (s sortState) Marker(column string) string {
	if s.Key != column {
		return ""
	}
	if s.Dir == sortDirDesc {
		return " ▼"
	}
	return " ▲"
}

const (
	usageDetailCost      = "cost"
	usageDetailDenials   = "denials"
	usageDetailFiles     = "files"
	usageDetailModels    = "models"
	usageDetailPlans     = "plans"
	usageDetailSkills    = "skills"
	usageDetailSubagents = "subagents"
)

func usageDetailParam(r *http.Request) string {
	switch detail := r.URL.Query().Get("detail"); detail {
	case usageDetailCost, usageDetailDenials, usageDetailFiles, usageDetailModels,
		usageDetailPlans, usageDetailSkills, usageDetailSubagents:
		return detail
	}
	return ""
}

func displayTotalTokens(usage *session.Usage) int {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.InputTokens + usage.OutputTokens +
		usage.CacheCreationInputTokens + usage.CacheReadInputTokens
}

func aggregateUsage(sess *session.Session) session.Usage {
	total := *sess.CurrentUsage()
	for _, stat := range sess.Subagents {
		total.Add(&stat.Usage)
	}
	return total
}

func cachePercent(agent session.Agent, usage *session.Usage) string {
	hit := usage.CacheReadInputTokens
	base := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	if agent == session.AgentCodex {
		hit = usage.CachedInputTokens
		base = usage.InputTokens
	}
	if base == 0 {
		return ""
	}
	return fmt.Sprintf("%.0f%%", float64(hit)/float64(base)*100)
}

type costRow struct {
	Component string
	Tokens    int
	Rate      string
	Cost      string
}

type costData struct {
	Id    session.Id
	Model string
	AsOf  string
	Known bool
	Rows  []costRow
	Total string

	totalValue float64
}

func newCostRow(component string, tokens int, ratePerMTok float64, total *float64) costRow {
	cost := pricing.Cost(tokens, ratePerMTok)
	*total += cost
	return costRow{
		Component: component,
		Tokens:    tokens,
		Rate:      fmt.Sprintf("$%.2f", ratePerMTok),
		Cost:      fmt.Sprintf("$%.4f", cost),
	}
}

func newCostData(id session.Id, agent session.Agent, model string, usage *session.Usage) costData {
	data := costData{Id: id, Model: model, AsOf: pricing.AsOf}
	rates, known := pricing.Lookup(model)
	if !known {
		return data
	}
	data.Known = true

	var total float64
	if agent == session.AgentCodex {
		uncached := max(0, usage.InputTokens-usage.CachedInputTokens)
		data.Rows = []costRow{
			newCostRow("Input (uncached)", uncached, rates.InputPerMTok, &total),
			newCostRow("Cached input", usage.CachedInputTokens, rates.CacheReadPerMTok, &total),
			newCostRow("Output", usage.OutputTokens, rates.OutputPerMTok, &total),
		}
	} else {
		// Writes without a tier breakdown (older transcripts) are priced at the
		// 5m rate, the API default TTL.
		untiered := max(0, usage.CacheCreationInputTokens-usage.CacheCreation5mInputTokens-usage.CacheCreation1hInputTokens)
		data.Rows = []costRow{
			newCostRow("Input", usage.InputTokens, rates.InputPerMTok, &total),
			newCostRow("Cache write (5m)", usage.CacheCreation5mInputTokens+untiered, rates.CacheWrite5mPerMTok, &total),
			newCostRow("Cache write (1h)", usage.CacheCreation1hInputTokens, rates.CacheWrite1hPerMTok, &total),
			newCostRow("Cache read", usage.CacheReadInputTokens, rates.CacheReadPerMTok, &total),
			newCostRow("Output", usage.OutputTokens, rates.OutputPerMTok, &total),
		}
	}
	data.Total = fmt.Sprintf("$%.4f", total)
	data.totalValue = total
	return data
}

func newSessionCostData(id session.Id, sess *session.Session) costData {
	data := newCostData(id, sess.Agent, sess.Meta.Model, sess.CurrentUsage())

	models := make(map[string]*session.Usage)
	counts := make(map[string]int)
	for _, stat := range sess.Subagents {
		model := subagentModel(stat, sess)
		if models[model] == nil {
			models[model] = &session.Usage{}
		}
		models[model].Add(&stat.Usage)
		counts[model]++
	}

	total := data.totalValue
	for _, model := range slices.Sorted(maps.Keys(models)) {
		group := newCostData(id, sess.Agent, model, models[model])
		row := costRow{
			Component: fmt.Sprintf("Subagents %s ×%d", model, counts[model]),
			Tokens:    displayTotalTokens(models[model]),
			Rate:      "—",
			Cost:      group.Total,
		}
		if !group.Known {
			row.Cost = "?"
		}
		data.Rows = append(data.Rows, row)
		total += group.totalValue
	}
	data.Total = fmt.Sprintf("$%.4f", total)
	data.totalValue = total
	return data
}

type planVersionRow struct {
	Index     int
	Timestamp time.Time
	Phase     string
	Delta     string
}

type planVersionsData struct {
	Id       session.Id
	Versions []planVersionRow
}

func newPlanVersionsData(sess *session.Session) *planVersionsData {
	data := &planVersionsData{Id: sess.Meta.SessionId}
	for _, revision := range sess.PlanRevisions {
		data.Versions = append(data.Versions, planVersionRow{
			Index:     revision.Index,
			Timestamp: revision.Timestamp,
			Phase:     revisionPhase(revision),
			Delta:     revisionDelta(revision),
		})
	}
	return data
}

func revisionPhase(revision *session.PlanRevision) string {
	if revision.Index == 0 {
		return "initial"
	}
	if revision.IsAlteration {
		return "alteration"
	}
	return "planning"
}

const maxRevisionDeltaLines = 999

func revisionDelta(revision *session.PlanRevision) string {
	if revision.Index == 0 {
		return "+" + truncatedLineCount(strings.Count(revision.Content, "\n")+1)
	}

	var added, removed int
	for line := range strings.Lines(revision.Diff) {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	if added == 0 && removed == 0 {
		return ""
	}
	return "+" + truncatedLineCount(added) + " −" + truncatedLineCount(removed)
}

func truncatedLineCount(count int) string {
	if count > maxRevisionDeltaLines {
		return "999+"
	}
	return strconv.Itoa(count)
}

type skillRow struct {
	Agent     string
	Skill     string
	StartedAt time.Time
	EndedAt   time.Time
	Duration  string
	Tokens    int
	Cost      string
}

type skillsData struct {
	Id     session.Id
	Sort   sortState
	Skills []skillRow
}

func skillAgentLabel(agentId string, sess *session.Session) string {
	if agentId == "" {
		return "main"
	}
	if stat, ok := sess.Subagents[agentId]; ok {
		return subagentTabLabel(agentId, stat)
	}
	runes := []rune(agentId)
	if len(runes) > 8 {
		return string(runes[:8])
	}
	return agentId
}

func newSkillsData(id session.Id, sess *session.Session, sort sortState) *skillsData {
	data := &skillsData{Id: id, Sort: sort}
	for _, skill := range sess.Skills {
		duration := "running"
		if !skill.EndedAt.IsZero() {
			duration = skill.EndedAt.Sub(skill.StartedAt).Round(time.Second).String()
		}
		model := skill.Model
		if model == "" {
			model = sess.Meta.Model
		}
		cost := newCostData(id, sess.Agent, model, &skill.Usage)
		data.Skills = append(data.Skills, skillRow{
			Agent:     skillAgentLabel(skill.AgentId, sess),
			Skill:     skill.Skill,
			StartedAt: skill.StartedAt,
			EndedAt:   skill.EndedAt,
			Duration:  duration,
			Tokens:    displayTotalTokens(&skill.Usage),
			Cost:      cost.Total,
		})
	}
	sortSkillRows(data.Skills, sort.Key, sort.Dir)
	return data
}

func sortSkillRows(rows []skillRow, key, dir string) {
	if key == "" {
		return
	}
	slices.SortFunc(rows, func(a, b skillRow) int {
		var c int
		switch key {
		case "agent":
			c = strings.Compare(a.Agent, b.Agent)
		case "skill":
			c = strings.Compare(a.Skill, b.Skill)
		case "started":
			c = a.StartedAt.Compare(b.StartedAt)
		case "ended":
			c = a.EndedAt.Compare(b.EndedAt)
		case "tokens":
			c = a.Tokens - b.Tokens
		}
		if dir == sortDirDesc {
			return -c
		}
		return c
	})
}

type subagentRow struct {
	Agent       string
	Description string
	Model       string
	StartedAt   time.Time
	LastActive  time.Time
	Duration    string
	Tokens      int
	Cost        string

	costValue float64
}

type subagentTableGroup struct {
	Name string
	Rows []subagentRow
}

type subagentsData struct {
	Id     session.Id
	Sort   sortState
	Groups []subagentTableGroup
}

func subagentModel(stat *session.SubagentStat, sess *session.Session) string {
	if stat.Model != "" {
		return stat.Model
	}
	return sess.Meta.Model
}

func newSubagentsData(id session.Id, sess *session.Session, sort sortState) *subagentsData {
	data := &subagentsData{Id: id, Sort: sort}
	byType := make(map[string]int)
	for _, agentId := range sess.SubagentIds() {
		stat := sess.Subagents[agentId]
		model := subagentModel(stat, sess)
		cost := newCostData(id, sess.Agent, model, &stat.Usage)
		row := subagentRow{
			Agent:       subagentTabLabel(agentId, stat),
			Description: stat.Description,
			Model:       model,
			StartedAt:   stat.FirstActive,
			LastActive:  stat.LastActive,
			Duration:    stat.LastActive.Sub(stat.FirstActive).Round(time.Second).String(),
			Tokens:      displayTotalTokens(&stat.Usage),
			Cost:        cost.Total,
			costValue:   cost.totalValue,
		}
		name := stat.AgentType
		if name == "" {
			name = "unknown"
		}
		index, ok := byType[name]
		if !ok {
			index = len(data.Groups)
			byType[name] = index
			data.Groups = append(data.Groups, subagentTableGroup{Name: name})
		}
		data.Groups[index].Rows = append(data.Groups[index].Rows, row)
	}
	slices.SortFunc(data.Groups, func(a, b subagentTableGroup) int { return strings.Compare(a.Name, b.Name) })
	for i := range data.Groups {
		sortSubagentRows(data.Groups[i].Rows, sort.Key, sort.Dir)
	}
	return data
}

func sortSubagentRows(rows []subagentRow, key, dir string) {
	sortKey := key
	if sortKey == "" {
		sortKey = "started"
		dir = sortDirAsc
	}
	slices.SortFunc(rows, func(a, b subagentRow) int {
		var c int
		switch sortKey {
		case "agent":
			c = strings.Compare(a.Agent, b.Agent)
		case "model":
			c = strings.Compare(a.Model, b.Model)
		case "started":
			c = a.StartedAt.Compare(b.StartedAt)
		case "lastactive":
			c = a.LastActive.Compare(b.LastActive)
		case "tokens":
			c = a.Tokens - b.Tokens
		case "cost":
			switch {
			case a.costValue < b.costValue:
				c = -1
			case a.costValue > b.costValue:
				c = 1
			}
		}
		if dir == sortDirDesc {
			return -c
		}
		return c
	})
}

type fileRow struct {
	Path   string
	Reads  int
	Writes int
}

type fileGroup struct {
	Ext   string
	Files []fileRow
}

type filesData struct {
	Id     session.Id
	Groups []fileGroup
	Config []fileRow
}

func fileExtension(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return "(none)"
	}
	return ext
}

func newFilesData(sess *session.Session) *filesData {
	data := &filesData{Id: sess.Meta.SessionId}
	byExt := make(map[string]int)
	for path, counts := range sess.TouchedFiles {
		row := fileRow{Path: path, Reads: counts.Reads, Writes: counts.Writes}
		if isClaudeConfigPath(path) {
			data.Config = append(data.Config, row)
			continue
		}
		ext := fileExtension(path)
		index, ok := byExt[ext]
		if !ok {
			index = len(data.Groups)
			byExt[ext] = index
			data.Groups = append(data.Groups, fileGroup{Ext: ext})
		}
		data.Groups[index].Files = append(data.Groups[index].Files, row)
	}
	byPath := func(a, b fileRow) int { return strings.Compare(a.Path, b.Path) }
	for i := range data.Groups {
		slices.SortFunc(data.Groups[i].Files, byPath)
	}
	slices.SortFunc(data.Config, byPath)
	slices.SortFunc(data.Groups, func(a, b fileGroup) int {
		if c := len(b.Files) - len(a.Files); c != 0 {
			return c
		}
		return strings.Compare(a.Ext, b.Ext)
	})
	return data
}

func isClaudeConfigPath(path string) bool {
	segments := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	for i, segment := range segments {
		if segment != ".claude" {
			continue
		}
		if i+1 < len(segments) && segments[i+1] == "worktrees" {
			continue
		}
		return true
	}
	return false
}

type modelRow struct {
	Timestamp time.Time
	From      string
	To        string
}

type modelsData struct {
	Id     session.Id
	Models []modelRow
}

func newModelsData(sess *session.Session) *modelsData {
	data := &modelsData{Id: sess.Meta.SessionId}
	all := sess.Events.All()
	slices.Reverse(all)
	for _, event := range all {
		if event.Kind != session.EventKindModelChanged || event.Model == nil {
			continue
		}
		data.Models = append(data.Models, modelRow{
			Timestamp: event.Timestamp,
			From:      event.Model.From,
			To:        event.Model.To,
		})
	}
	return data
}

type denialRow struct {
	Tool      string
	Command   string
	Timestamp time.Time
}

type denialsData struct {
	Id      session.Id
	Denials []denialRow
}

func newDenialsData(sess *session.Session) *denialsData {
	data := &denialsData{Id: sess.Meta.SessionId}
	all := sess.Events.All()
	slices.Reverse(all)
	for _, event := range all {
		if event.Kind != session.EventKindPermissionDenied || event.Permission == nil {
			continue
		}
		data.Denials = append(data.Denials, denialRow{
			Tool:      event.Permission.Tool,
			Command:   event.Permission.Command,
			Timestamp: event.Timestamp,
		})
	}
	return data
}
