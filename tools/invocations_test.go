package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvocationCounter(t *testing.T) {
	counter := NewInvocationCounter(InstanceInfo{PID: 1, StartedAt: time.Now()}, nil)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.Inc("session_list", 10)
		}()
	}
	wg.Wait()
	counter.Inc("session_get", 3)

	snapshot := counter.Snapshot()
	assert.Equal(t, ToolStats{Count: 50, Bytes: 500}, snapshot["session_list"])
	assert.Equal(t, ToolStats{Count: 1, Bytes: 3}, snapshot["session_get"])

	snapshot["session_get"] = ToolStats{Count: 99}
	assert.Equal(t, ToolStats{Count: 1, Bytes: 3}, counter.Snapshot()["session_get"])
}

func TestInvocationCounterPersist(t *testing.T) {
	root := t.TempDir()
	startedAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	info := InstanceInfo{PID: 4242, PPID: 1, Transport: "stdio", Version: "test", StartedAt: startedAt}
	counter := NewInvocationCounter(info, state.NewDir(root))

	counter.Persist()
	counter.Inc("session_list", 7)
	counter.AddClient("claude-code 2.0")
	counter.AddClient("claude-code 2.0")
	counter.AddClient("")

	path := filepath.Join(root, "instances", fmt.Sprintf("%d-4242.json", startedAt.Unix()))
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var record InstanceRecord
	require.NoError(t, json.Unmarshal(data, &record))
	assert.Equal(t, fmt.Sprintf("%d-4242", startedAt.Unix()), record.Id)
	assert.Equal(t, 4242, record.PID)
	assert.Equal(t, 1, record.PPID)
	assert.Equal(t, "stdio", record.Transport)
	assert.Equal(t, "test", record.Version)
	assert.Equal(t, []string{"claude-code 2.0"}, record.Clients)
	assert.Equal(t, ToolStats{Count: 1, Bytes: 7}, record.Tools["session_list"])
	assert.False(t, record.UpdatedAt.IsZero())
}
