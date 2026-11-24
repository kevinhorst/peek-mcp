package index

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kevinhorst/peek-mcp/session"
)

func TestAgentTypeMedians(t *testing.T) {
	type testCase struct {
		_id       string
		_expected []*AgentTypeMedian

		runs []*SessionRun
	}

	agent := func(agentType string, model string, count int, usage session.Usage) *session.AgentTypeUsage {
		return &session.AgentTypeUsage{AgentType: agentType, Count: count, Model: model, Usage: usage}
	}
	runOf := func(agents ...*session.AgentTypeUsage) *SessionRun {
		return &SessionRun{Record: &Record{}, Run: &Run{Agents: agents}}
	}

	tests := make([]*testCase, 0)

	// no-runs
	tests = append(tests, &testCase{
		_id:       "no-runs",
		_expected: []*AgentTypeMedian{},
		runs:      nil,
	})

	// one-run-divides-by-count
	oneRunUsage := session.Usage{CacheCreationInputTokens: 300, CacheReadInputTokens: 600, InputTokens: 30, OutputTokens: 90}
	oneRunPerAgent := session.Usage{CacheCreationInputTokens: 100, CacheReadInputTokens: 200, InputTokens: 10, OutputTokens: 30}
	tests = append(tests, &testCase{
		_id:       "one-run-divides-by-count",
		_expected: []*AgentTypeMedian{{AgentType: "lane", CountMedian: 3, Model: "opus", PerAgent: oneRunPerAgent, Runs: 1}},
		runs:      []*SessionRun{runOf(agent("lane", "opus", 3, oneRunUsage))},
	})

	// three-runs-upper-median
	threeRunsPerAgent := session.Usage{CacheCreationInputTokens: 20, CacheReadInputTokens: 200, InputTokens: 1, OutputTokens: 30}
	tests = append(tests, &testCase{
		_id:       "three-runs-upper-median",
		_expected: []*AgentTypeMedian{{AgentType: "lane", CountMedian: 2, Model: "opus", PerAgent: threeRunsPerAgent, Runs: 3}},
		runs: []*SessionRun{
			runOf(agent("lane", "opus", 1, session.Usage{CacheCreationInputTokens: 10, CacheReadInputTokens: 300, InputTokens: 3, OutputTokens: 10})),
			runOf(agent("lane", "opus", 2, session.Usage{CacheCreationInputTokens: 40, CacheReadInputTokens: 400, InputTokens: 2, OutputTokens: 60})),
			runOf(agent("lane", "opus", 4, session.Usage{CacheCreationInputTokens: 120, CacheReadInputTokens: 400, InputTokens: 4, OutputTokens: 160})),
		},
	})

	// two-runs-upper-median
	tests = append(tests, &testCase{
		_id:       "two-runs-upper-median",
		_expected: []*AgentTypeMedian{{AgentType: "lane", CountMedian: 3, Model: "opus", PerAgent: session.Usage{OutputTokens: 30}, Runs: 2}},
		runs: []*SessionRun{
			runOf(agent("lane", "opus", 1, session.Usage{OutputTokens: 10})),
			runOf(agent("lane", "opus", 3, session.Usage{OutputTokens: 90})),
		},
	})

	// count-zero-divides-by-one
	tests = append(tests, &testCase{
		_id:       "count-zero-divides-by-one",
		_expected: []*AgentTypeMedian{{AgentType: "lane", CountMedian: 0, Model: "opus", PerAgent: session.Usage{OutputTokens: 50}, Runs: 1}},
		runs:      []*SessionRun{runOf(agent("lane", "opus", 0, session.Usage{OutputTokens: 50}))},
	})

	// two-models-two-rows-sorted
	tests = append(tests, &testCase{
		_id: "two-models-two-rows-sorted",
		_expected: []*AgentTypeMedian{
			{AgentType: "critical", CountMedian: 1, Model: "sonnet", PerAgent: session.Usage{OutputTokens: 5}, Runs: 1},
			{AgentType: "lane", CountMedian: 1, Model: "opus", PerAgent: session.Usage{OutputTokens: 20}, Runs: 1},
			{AgentType: "lane", CountMedian: 1, Model: "sonnet", PerAgent: session.Usage{OutputTokens: 10}, Runs: 2},
		},
		runs: []*SessionRun{
			runOf(agent("lane", "sonnet", 1, session.Usage{OutputTokens: 10}), agent("lane", "opus", 1, session.Usage{OutputTokens: 20})),
			runOf(agent("lane", "sonnet", 1, session.Usage{OutputTokens: 10}), agent("critical", "sonnet", 1, session.Usage{OutputTokens: 5})),
		},
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			assert.Equal(t, test._expected, AgentTypeMedians(test.runs))
		})
	}
}
