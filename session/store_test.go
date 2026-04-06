package session

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetOrCreate_New(t *testing.T) {
	s := New(10)
	sess := s.GetOrCreate("s1", "claude")

	assert.Equal(t, Id("s1"), sess.Info.Id)
	assert.Equal(t, SourceClaude, sess.Info.Source)
	assert.NotNil(t, sess.Turns)
}

func TestGetOrCreate_Existing(t *testing.T) {
	s := New(10)
	s1 := s.GetOrCreate("s1", "claude")
	s2 := s.GetOrCreate("s1", "claude")

	assert.Same(t, s1, s2)
}

func TestGet_NotFound(t *testing.T) {
	s := New(10)
	_, ok := s.Get("nonexistent")
	assert.False(t, ok)
}

func TestGet_Found(t *testing.T) {
	s := New(10)
	s.GetOrCreate("s1", "codex")

	sess, ok := s.Get("s1")
	assert.True(t, ok)
	assert.Equal(t, Id("s1"), sess.Info.Id)
}

func TestList_Empty(t *testing.T) {
	s := New(10)
	assert.Empty(t, s.List())
}

func TestList_SortedByLastActive(t *testing.T) {
	s := New(10)
	now := time.Now()

	s1 := s.GetOrCreate("s1", "claude")
	s1.Info.LastActive = now.Add(-2 * time.Hour)

	s2 := s.GetOrCreate("s2", "codex")
	s2.Info.LastActive = now

	s3 := s.GetOrCreate("s3", "claude")
	s3.Info.LastActive = now.Add(-1 * time.Hour)

	list := s.List()
	assert.Len(t, list, 3)
	assert.Equal(t, Id("s2"), list[0].Id)
	assert.Equal(t, Id("s3"), list[1].Id)
	assert.Equal(t, Id("s1"), list[2].Id)
}

func TestMostRecent_Empty(t *testing.T) {
	s := New(10)
	_, ok := s.Last()
	assert.False(t, ok)
}

func TestMostRecent(t *testing.T) {
	s := New(10)
	now := time.Now()

	s1 := s.GetOrCreate("s1", "claude")
	s1.Info.LastActive = now.Add(-1 * time.Hour)

	s2 := s.GetOrCreate("s2", "codex")
	s2.Info.LastActive = now

	sess, ok := s.Last()
	assert.True(t, ok)
	assert.Equal(t, Id("s2"), sess.Info.Id)
}

func TestConcurrentAccess(t *testing.T) {
	s := New(10)
	var wg sync.WaitGroup

	// Concurrent readers and writers. Writers only use GetOrCreate (which holds the lock).
	// This matches real usage: the watcher serializes all meta/turn writes behind its own mutex.
	for i := 0; i < 50; i++ {
		id := Id("session-" + string(rune('a'+i%10)))

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.GetOrCreate(id, "claude")
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.List()
			s.Last()
		}()
	}

	wg.Wait()
}
