package tools

import (
	"maps"
	"sync"
)

type InvocationCounter struct {
	mu     sync.Mutex
	counts map[string]int
}

func NewInvocationCounter() *InvocationCounter {
	return &InvocationCounter{counts: make(map[string]int)}
}

func (c *InvocationCounter) Inc(tool string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[tool]++
}

func (c *InvocationCounter) Snapshot() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.counts)
}
