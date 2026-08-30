package control

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kevinhorst/peek-mcp/pricing"
	"github.com/kevinhorst/peek-mcp/session"
)

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
	Known bool
	Rows  []costRow
	Total string
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
	data := costData{Id: id, Model: model}
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
		data.Rows = []costRow{
			newCostRow("Input", usage.InputTokens, rates.InputPerMTok, &total),
			newCostRow("Cache write", usage.CacheCreationInputTokens, rates.CacheWritePerMTok, &total),
			newCostRow("Cache read", usage.CacheReadInputTokens, rates.CacheReadPerMTok, &total),
			newCostRow("Output", usage.OutputTokens, rates.OutputPerMTok, &total),
		}
	}
	data.Total = fmt.Sprintf("$%.4f", total)
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
	Skill     string
	StartedAt time.Time
	Duration  string
	Tokens    int
	Cost      string
}

type skillsData struct {
	Id     session.Id
	Skills []skillRow
}

func newSkillsData(id session.Id, sess *session.Session) *skillsData {
	data := &skillsData{Id: id}
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
			Skill:     skill.Skill,
			StartedAt: skill.StartedAt,
			Duration:  duration,
			Tokens:    displayTotalTokens(&skill.Usage),
			Cost:      cost.Total,
		})
	}
	return data
}

type subagentRow struct {
	Agent       string
	Description string
	Model       string
	StartedAt   time.Time
	Duration    string
	Tokens      int
	Cost        string
}

type subagentsData struct {
	Id        session.Id
	Subagents []subagentRow
}

func subagentModel(stat *session.SubagentStat, sess *session.Session) string {
	if stat.Model != "" {
		return stat.Model
	}
	return sess.Meta.Model
}

func newSubagentsData(id session.Id, sess *session.Session) *subagentsData {
	data := &subagentsData{Id: id}
	for _, stat := range sess.Subagents {
		model := subagentModel(stat, sess)
		cost := newCostData(id, sess.Agent, model, &stat.Usage)
		data.Subagents = append(data.Subagents, subagentRow{
			Agent:       stat.AgentType,
			Description: stat.Description,
			Model:       model,
			StartedAt:   stat.FirstActive,
			Duration:    stat.LastActive.Sub(stat.FirstActive).Round(time.Second).String(),
			Tokens:      displayTotalTokens(&stat.Usage),
			Cost:        cost.Total,
		})
	}
	slices.SortFunc(data.Subagents, func(a, b subagentRow) int { return a.StartedAt.Compare(b.StartedAt) })
	return data
}

type fileRow struct {
	Path   string
	Reads  int
	Writes int
}

type filesData struct {
	Id    session.Id
	Files []fileRow
}

func newFilesData(sess *session.Session) *filesData {
	data := &filesData{Id: sess.Meta.SessionId}
	for path, counts := range sess.TouchedFiles {
		data.Files = append(data.Files, fileRow{Path: path, Reads: counts.Reads, Writes: counts.Writes})
	}
	slices.SortFunc(data.Files, func(a, b fileRow) int { return strings.Compare(a.Path, b.Path) })
	return data
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
