package claude

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLine_FileTouches(t *testing.T) {
	// write-yields-write-touch
	p := NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Write","input":{"file_path":"/repo/a.go","content":"x"}}]}}`))
	turn := p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","is_error":false,"content":"ok"}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.FileTouches, 1)
	assert.Equal(t, "/repo/a.go", turn.FileTouches[0].Path)
	assert.True(t, turn.FileTouches[0].Write)

	// read-yields-read-touch
	p = NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu2","name":"Read","input":{"file_path":"/repo/b.go"}}]}}`))
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu2","is_error":false,"content":"contents"}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.FileTouches, 1)
	assert.Equal(t, "/repo/b.go", turn.FileTouches[0].Path)
	assert.False(t, turn.FileTouches[0].Write)

	// notebook-path-fallback
	p = NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu3","name":"NotebookEdit","input":{"notebook_path":"/repo/n.ipynb"}}]}}`))
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu3","is_error":false,"content":"ok"}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.FileTouches, 1)
	assert.Equal(t, "/repo/n.ipynb", turn.FileTouches[0].Path)
	assert.True(t, turn.FileTouches[0].Write)

	// error-result-no-touch
	p = NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu4","name":"Write","input":{"file_path":"/repo/c.go"}}]}}`))
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu4","is_error":true,"content":"The user doesn't want to proceed with this tool use."}]}}`))
	require.NotNil(t, turn)
	assert.Empty(t, turn.FileTouches)
	require.Len(t, turn.Events, 1)

	// non-file-tool-no-touch
	p = NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu5","name":"Bash","input":{"command":"ls"}}]}}`))
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu5","is_error":false,"content":"ok"}]}}`))
	assert.Nil(t, turn)

	// sidechain-touch-collected
	p = NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","agentId":"sub-ag","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":true,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu6","name":"Edit","input":{"file_path":"/repo/d.go"}}]}}`))
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","agentId":"sub-ag","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":true,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu6","is_error":false,"content":"ok"}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.FileTouches, 1)
	assert.Equal(t, "/repo/d.go", turn.FileTouches[0].Path)
}

func TestParseLine_SidechainSubagentTurn(t *testing.T) {
	// assistant-carries-usage-and-identity
	p := NewParser()
	turn := p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","agentId":"sub-ag","requestId":"req-1","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":true,"message":{"role":"assistant","content":[{"type":"text","text":"working"}],"usage":{"input_tokens":11,"output_tokens":4}}}`))
	require.NotNil(t, turn)
	assert.Equal(t, "sub-ag", turn.SubagentId)
	assert.Equal(t, "req-1", turn.RequestId)
	assert.False(t, turn.Timestamp.IsZero())
	require.NotNil(t, turn.Usage)
	assert.Equal(t, 11, turn.Usage.InputTokens)
	assert.Equal(t, 4, turn.Usage.OutputTokens)

	// user-entry-no-usage
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","agentId":"sub-ag","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":true,"message":{"role":"user","content":"next step"}}`))
	require.NotNil(t, turn)
	assert.Equal(t, "sub-ag", turn.SubagentId)
	assert.Nil(t, turn.Usage)
}
