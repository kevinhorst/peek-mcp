package tools

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/kevinhorst/peek-mcp/state"
)

type ToolStats struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}

type InstanceInfo struct {
	Id        string    `json:"id"`
	PID       int       `json:"pid"`
	PPID      int       `json:"ppid"`
	Transport string    `json:"transport"`
	Version   string    `json:"version"`
	StartedAt time.Time `json:"started_at"`
}

type InstanceRecord struct {
	InstanceInfo
	Clients   []string             `json:"clients,omitempty"`
	Tools     map[string]ToolStats `json:"tools,omitempty"`
	UpdatedAt time.Time            `json:"updated_at"`
}

type InvocationCounter struct {
	mu       sync.Mutex
	info     InstanceInfo
	clients  []string
	counts   map[string]ToolStats
	stateDir *state.Dir
}

func NewInvocationCounter(info InstanceInfo, stateDir *state.Dir) *InvocationCounter {
	info.Id = fmt.Sprintf("%d-%d", info.StartedAt.Unix(), info.PID)
	return &InvocationCounter{info: info, counts: make(map[string]ToolStats), stateDir: stateDir}
}

func (c *InvocationCounter) Inc(tool string, bytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stats := c.counts[tool]
	stats.Count++
	stats.Bytes += bytes
	c.counts[tool] = stats
	c.persist()
}

func (c *InvocationCounter) AddClient(client string) {
	if client == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if slices.Contains(c.clients, client) {
		return
	}
	c.clients = append(c.clients, client)
	c.persist()
}

func (c *InvocationCounter) Snapshot() map[string]ToolStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.counts)
}

func (c *InvocationCounter) Persist() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.persist()
}

func (c *InvocationCounter) persist() {
	if c.stateDir == nil {
		return
	}
	record := InstanceRecord{
		InstanceInfo: c.info,
		Clients:      slices.Clone(c.clients),
		Tools:        maps.Clone(c.counts),
		UpdatedAt:    time.Now(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	_ = c.stateDir.WriteInstance(c.info.Id, string(data))
}
