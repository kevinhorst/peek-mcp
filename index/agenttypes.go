package index

import (
	"cmp"
	"slices"

	"github.com/kevinhorst/peek-mcp/session"
)

// AgentTypeMedian is one agent type and model across a skill's runs: how many runs had it, its median count per run, the median usage of one agent.
type AgentTypeMedian struct {
	AgentType   string        `json:"agent_type"`
	CountMedian int           `json:"count_median"`
	Model       string        `json:"model,omitempty"`
	PerAgent    session.Usage `json:"per_agent"`
	Runs        int           `json:"runs"`
}

type agentTypeKey struct {
	agentType string
	model     string
}

// AgentTypeMedians groups the runs' agents by agent type and model, sorted by both.
func AgentTypeMedians(runs []*SessionRun) []*AgentTypeMedian {
	rowsByKey := map[agentTypeKey][]*session.AgentTypeUsage{}
	for _, sessionRun := range runs {
		for _, agentType := range sessionRun.Run.Agents {
			key := agentTypeKey{agentType: agentType.AgentType, model: agentType.Model}
			rowsByKey[key] = append(rowsByKey[key], agentType)
		}
	}

	medians := make([]*AgentTypeMedian, 0, len(rowsByKey))
	for key, rows := range rowsByKey {
		medians = append(medians, newAgentTypeMedian(key, rows))
	}
	slices.SortFunc(medians, compareAgentTypes)
	return medians
}

func compareAgentTypes(left, right *AgentTypeMedian) int {
	return cmp.Or(cmp.Compare(left.AgentType, right.AgentType), cmp.Compare(left.Model, right.Model))
}

// median is the upper median of the values.
func median(values []int) int {
	sorted := slices.Sorted(slices.Values(values))
	return sorted[len(sorted)/2]
}

// newAgentTypeMedian: per run one agent's usage is the row's usage over its count; each field is the median across runs.
func newAgentTypeMedian(key agentTypeKey, rows []*session.AgentTypeUsage) *AgentTypeMedian {
	counts := make([]int, 0, len(rows))
	cacheCreation := make([]int, 0, len(rows))
	cacheRead := make([]int, 0, len(rows))
	input := make([]int, 0, len(rows))
	output := make([]int, 0, len(rows))
	for _, row := range rows {
		count := max(row.Count, 1)
		counts = append(counts, row.Count)
		cacheCreation = append(cacheCreation, row.Usage.CacheCreationInputTokens/count)
		cacheRead = append(cacheRead, row.Usage.CacheReadInputTokens/count)
		input = append(input, row.Usage.InputTokens/count)
		output = append(output, row.Usage.OutputTokens/count)
	}

	perAgent := session.Usage{
		CacheCreationInputTokens: median(cacheCreation),
		CacheReadInputTokens:     median(cacheRead),
		InputTokens:              median(input),
		OutputTokens:             median(output),
	}
	return &AgentTypeMedian{AgentType: key.agentType, CountMedian: median(counts), Model: key.model, PerAgent: perAgent, Runs: len(rows)}
}
