package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSnapshotCache(t *testing.T) {
	// put-get-round-trip
	t.Run("put-get-round-trip", func(t *testing.T) {
		cache := newSnapshotCache(2)
		cache.put("s1", "diff one")

		content, ok := cache.get("s1")
		assert.True(t, ok)
		assert.Equal(t, "diff one", content)
	})

	// eviction-drops-least-recently-used
	t.Run("eviction-drops-least-recently-used", func(t *testing.T) {
		cache := newSnapshotCache(2)
		cache.put("s1", "diff one")
		cache.put("s2", "diff two")
		cache.put("s3", "diff three")

		_, ok := cache.get("s1")
		assert.False(t, ok)
		_, ok = cache.get("s2")
		assert.True(t, ok)
		_, ok = cache.get("s3")
		assert.True(t, ok)
	})

	// get-refreshes-recency
	t.Run("get-refreshes-recency", func(t *testing.T) {
		cache := newSnapshotCache(2)
		cache.put("s1", "diff one")
		cache.put("s2", "diff two")
		cache.get("s1")
		cache.put("s3", "diff three")

		_, ok := cache.get("s1")
		assert.True(t, ok)
		_, ok = cache.get("s2")
		assert.False(t, ok)
	})

	// update-existing-keeps-size
	t.Run("update-existing-keeps-size", func(t *testing.T) {
		cache := newSnapshotCache(2)
		cache.put("s1", "diff one")
		cache.put("s2", "diff two")
		cache.put("s1", "diff one updated")

		content, ok := cache.get("s1")
		assert.True(t, ok)
		assert.Equal(t, "diff one updated", content)
		_, ok = cache.get("s2")
		assert.True(t, ok)
		assert.Equal(t, 2, cache.order.Len())
	})

	// zero-capacity-disables
	t.Run("zero-capacity-disables", func(t *testing.T) {
		cache := newSnapshotCache(0)
		cache.put("s1", "diff one")

		_, ok := cache.get("s1")
		assert.False(t, ok)
	})
}
