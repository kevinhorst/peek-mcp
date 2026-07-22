package codex

import (
	"os"
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const escalatedCallLine = `{"timestamp":"2026-03-29T23:45:23.000Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"c1","arguments":"{\"cmd\":\"rm -rf /tmp/x\",\"justification\":\"cleanup\",\"sandbox_permissions\":\"require_escalated\"}"}}`

func subagentParser(t *testing.T) *Parser {
	t.Helper()
	p := NewParser()
	data, err := os.ReadFile("fixtures/subagent_meta.json")
	require.NoError(t, err)
	turn := p.ParseLine(data)
	require.NotNil(t, turn)
	return p
}

func TestParseLine_PermissionEvents(t *testing.T) {
	// escalated-denied-event
	p := seededParser(t)
	assert.Nil(t, p.ParseLine([]byte(escalatedCallLine)))
	turn := p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:24.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c1","output":"exec failed: Rejected(\"rejected by user\")"}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindPermissionDenied, turn.Events[0].Kind)
	assert.Equal(t, "exec_command", turn.Events[0].Permission.Tool)
	assert.Equal(t, "rm -rf /tmp/x", turn.Events[0].Permission.Command)
	assert.Equal(t, "cleanup", turn.Events[0].Permission.Justification)

	// escalated-granted-no-event
	p = seededParser(t)
	assert.Nil(t, p.ParseLine([]byte(escalatedCallLine)))
	turn = p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:24.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c1","output":"ok, done"}}`))
	assert.Nil(t, turn, "grants stay implicit")

	// non-escalated-failure-no-event
	p = seededParser(t)
	assert.Nil(t, p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:23.000Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"c2","arguments":"{\"cmd\":\"ls\",\"sandbox_permissions\":\"read-only\"}"}}`)))
	turn = p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:24.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c2","output":"exec failed: Rejected(\"rejected by user\")"}}`))
	assert.Nil(t, turn, "non-escalated calls are not tracked")

	// object-arguments-not-tracked
	p = seededParser(t)
	assert.Nil(t, p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:23.000Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"c3","arguments":{"cmd":"ls","sandbox_permissions":"require_escalated"}}}`)))
	turn = p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:24.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c3","output":"exec failed: Rejected(\"rejected by user\")"}}`))
	assert.Nil(t, turn, "object-form arguments are not the exec_command escalation shape")

	// object-arguments-do-not-break-the-parser
	p = seededParser(t)
	assert.Nil(t, p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:23.000Z","type":"response_item","payload":{"type":"tool_search_call","call_id":"c4","arguments":{"query":"select:Read"}}}`)))
	assert.Nil(t, p.ParseLine([]byte(escalatedCallLine)))
	turn = p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:24.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c1","output":"exec failed: Rejected(\"rejected by user\")"}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindPermissionDenied, turn.Events[0].Kind)
}

func TestParseLine_SubagentRollout(t *testing.T) {
	// spawned-event-with-parent
	t.Run("spawned-event-with-parent", func(t *testing.T) {
		p := NewParser()
		data, err := os.ReadFile("fixtures/subagent_meta.json")
		require.NoError(t, err)
		turn := p.ParseLine(data)
		require.NotNil(t, turn)
		require.True(t, turn.IsSubagentSignal())
		require.Len(t, turn.Events, 1)
		assert.Equal(t, session.EventKindSubagentSpawned, turn.Events[0].Kind)
		assert.Equal(t, "Hume", turn.Events[0].Actor)
		assert.Equal(t, "sess-codex-sub", turn.Events[0].Subagent.AgentId)
		assert.Equal(t, session.Id("sess-codex-parent"), turn.Meta.SessionId)
	})

	// no-parent-dropped
	t.Run("no-parent-dropped", func(t *testing.T) {
		p := NewParser()
		line := []byte(`{"timestamp":"2026-03-29T23:45:22.000Z","type":"session_meta","payload":{"id":"sub-x","source":{"subagent":{"thread_spawn":{"agent_nickname":"Nemo"}}}}}`)
		assert.Nil(t, p.ParseLine(line))
	})

	// chat-and-usage-suppressed-in-subagent-mode
	t.Run("chat-and-usage-suppressed-in-subagent-mode", func(t *testing.T) {
		p := subagentParser(t)
		assert.Nil(t, p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:23.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}`)))
		assert.Nil(t, p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:24.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}}`)))
	})

	// escalated-denial-carries-actor
	t.Run("escalated-denial-carries-actor", func(t *testing.T) {
		p := subagentParser(t)
		assert.Nil(t, p.ParseLine([]byte(escalatedCallLine)))
		turn := p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:24.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c1","output":"exec failed: Rejected(\"rejected by user\")"}}`))
		require.NotNil(t, turn)
		require.Len(t, turn.Events, 1)
		assert.Equal(t, session.EventKindPermissionDenied, turn.Events[0].Kind)
		assert.Equal(t, "Hume", turn.Events[0].Actor)
		assert.Equal(t, session.Id("sess-codex-parent"), turn.Meta.SessionId)
	})
}
