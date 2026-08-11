package control

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/kevinhorst/peek-mcp/pricing"
	"github.com/kevinhorst/peek-mcp/session"
)

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

func (s *Server) handleUsageCostFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	var data costData
	if !s.store.WithSession(id, func(sess *session.Session) {
		data = newCostData(id, sess.Agent, sess.Meta.Model, sess.CurrentUsage())
	}) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplUsageCost, data)
}

type planVersionRow struct {
	Index      int
	Timestamp  time.Time
	Alteration bool
}

type planVersionsData struct {
	Id       session.Id
	Versions []planVersionRow
}

func (s *Server) handleUsagePlansFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	data := planVersionsData{Id: id}
	if !s.store.WithSession(id, func(sess *session.Session) {
		for _, revision := range sess.PlanRevisions {
			data.Versions = append(data.Versions, planVersionRow{
				Index:      revision.Index,
				Timestamp:  revision.Timestamp,
				Alteration: revision.IsAlteration,
			})
		}
	}) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplUsagePlans, data)
}

type skillRow struct {
	Skill     string
	StartedAt time.Time
	Duration  string
	Tokens    int
}

type skillsData struct {
	Id     session.Id
	Skills []skillRow
}

func (s *Server) handleUsageSkillsFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	data := skillsData{Id: id}
	if !s.store.WithSession(id, func(sess *session.Session) {
		for _, skill := range sess.Skills {
			duration := "running"
			if !skill.EndedAt.IsZero() {
				duration = skill.EndedAt.Sub(skill.StartedAt).Round(time.Second).String()
			}
			data.Skills = append(data.Skills, skillRow{
				Skill:     skill.Skill,
				StartedAt: skill.StartedAt,
				Duration:  duration,
				Tokens:    displayTotalTokens(&skill.Usage),
			})
		}
	}) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplUsageSkills, data)
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

func (s *Server) handleUsageDenialsFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	data := denialsData{Id: id}
	if !s.store.WithSession(id, func(sess *session.Session) {
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
	}) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplUsageDenials, data)
}
