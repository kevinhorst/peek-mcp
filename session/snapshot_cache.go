package session

import (
	"container/list"
	"sync"
)

type snapshotCache struct {
	capacity int
	content  map[Id]string
	entries  map[Id]*list.Element
	mu       sync.Mutex
	order    *list.List
}

func newSnapshotCache(capacity int) *snapshotCache {
	cache := &snapshotCache{
		capacity: capacity,
		content:  make(map[Id]string),
		entries:  make(map[Id]*list.Element),
		order:    list.New(),
	}
	return cache
}

func (c *snapshotCache) get(id Id) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.entries[id]
	if !ok {
		return "", false
	}

	c.order.MoveToFront(element)
	return c.content[id], true
}

func (c *snapshotCache) put(id Id, snapshot string) {
	if c.capacity <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.entries[id]; ok {
		c.order.MoveToFront(element)
		c.content[id] = snapshot
		return
	}

	c.entries[id] = c.order.PushFront(id)
	c.content[id] = snapshot
	if c.order.Len() > c.capacity {
		oldest := c.order.Back()
		evictedId := oldest.Value.(Id)
		c.order.Remove(oldest)
		delete(c.entries, evictedId)
		delete(c.content, evictedId)
	}
}
