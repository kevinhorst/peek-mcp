package claude

import (
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLine_ToolCalls(t *testing.T) {
	// assistant-tool-use-yields-call
	p := NewParser()
	turn := p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","requestId":"r1","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Read","input":{"file_path":"/repo/a.go"}}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.ToolCalls, 1)
	assert.Equal(t, "tu1", turn.ToolCalls[0].Id)
	assert.Equal(t, "Read", turn.ToolCalls[0].Name)
	assert.Equal(t, turn.Timestamp, turn.ToolCalls[0].Timestamp)
	assert.False(t, turn.ToolCalls[0].IsError)

	// parallel-blocks-keep-order
	p = NewParser()
	turn = p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","requestId":"r1","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Read","input":{}},{"type":"tool_use","id":"tu2","name":"Grep","input":{}},{"type":"tool_use","id":"tu3","name":"Glob","input":{}}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.ToolCalls, 3)
	assert.Equal(t, "tu1", turn.ToolCalls[0].Id)
	assert.Equal(t, "tu2", turn.ToolCalls[1].Id)
	assert.Equal(t, "tu3", turn.ToolCalls[2].Id)

	// text-and-tool-use-same-turn
	p = NewParser()
	turn = p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","requestId":"r1","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"Reading it."},{"type":"tool_use","id":"tu1","name":"Read","input":{}}]}}`))
	require.NotNil(t, turn)
	assert.Equal(t, "Reading it.\n", turn.Text)
	assert.Len(t, turn.ToolCalls, 1)

	// input-kept-verbatim
	p = NewParser()
	turn = p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","requestId":"r1","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Bash","input":{"command":"ls -la","description":"list"}}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.ToolCalls, 1)
	assert.Equal(t, `{"command":"ls -la","description":"list"}`, string(turn.ToolCalls[0].Input))

	// sidechain-assistant-carries-calls
	p = NewParser()
	turn = p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","agentId":"sub-ag","requestId":"r1","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":true,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-sub","name":"Edit","input":{}}]}}`))
	require.NotNil(t, turn)
	assert.Equal(t, "sub-ag", turn.SubagentId)
	require.Len(t, turn.ToolCalls, 1)
	assert.Equal(t, "tu-sub", turn.ToolCalls[0].Id)

	// text-only-no-calls
	p = NewParser()
	turn = p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","requestId":"r1","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`))
	require.NotNil(t, turn)
	assert.Nil(t, turn.ToolCalls)
}

func TestParseLine_ToolResults(t *testing.T) {
	// main-chain-result-yields-signal-turn
	p := NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Bash","input":{"command":"ls"}}]}}`))
	turn := p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","is_error":false,"content":"ok"}]}}`))
	require.NotNil(t, turn)
	assert.Equal(t, session.Role(""), turn.Role)
	assert.NoError(t, turn.Validate())
	require.Len(t, turn.ToolResults, 1)
	assert.Equal(t, "tu1", turn.ToolResults[0].ToolUseId)
	assert.False(t, turn.ToolResults[0].IsError)

	// error-flag-carried
	p = NewParser()
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu2","is_error":true,"content":"exit status 1"}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.ToolResults, 1)
	assert.True(t, turn.ToolResults[0].IsError)

	// null-is-error-is-false
	p = NewParser()
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu3","is_error":null,"content":"ok"}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.ToolResults, 1)
	assert.False(t, turn.ToolResults[0].IsError)

	// result-without-pending-use-still-emitted
	p = NewParser()
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"never-seen","is_error":false,"content":"ok"}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.ToolResults, 1)
	assert.Equal(t, "never-seen", turn.ToolResults[0].ToolUseId)

	// sidechain-user-carries-results
	p = NewParser()
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","agentId":"sub-ag","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":true,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-sub","is_error":true,"content":"failed"}]}}`))
	require.NotNil(t, turn)
	assert.Equal(t, "sub-ag", turn.SubagentId)
	require.Len(t, turn.ToolResults, 1)
	assert.Equal(t, "tu-sub", turn.ToolResults[0].ToolUseId)
	assert.True(t, turn.ToolResults[0].IsError)

	// string-content-prompt-no-results
	p = NewParser()
	turn = p.ParseLine([]byte(`{"type":"user","promptId":"p1","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":"hello"}}`))
	require.NotNil(t, turn)
	assert.Nil(t, turn.ToolResults)
}
