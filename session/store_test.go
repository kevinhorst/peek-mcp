package session

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetOrCreate_New(t *testing.T) {
	s := NewStore(10)
	sess := s.getOrCreate("s1", AgentClaude)

	assert.Equal(t, Id("s1"), sess.Meta.SessionId)
	assert.Equal(t, AgentClaude, sess.Agent)
	assert.NotNil(t, sess.TurnsFinished)
}

func TestGetOrCreate_Existing(t *testing.T) {
	s := NewStore(10)
	s1 := s.getOrCreate("s1", AgentClaude)
	s2 := s.getOrCreate("s1", AgentClaude)

	assert.Same(t, s1, s2)
}

func TestGet_NotFound(t *testing.T) {
	s := NewStore(10)
	_, ok := s.GetById("nonexistent")
	assert.False(t, ok)
}

func TestGet_Found(t *testing.T) {
	s := NewStore(10)
	s.getOrCreate("s1", AgentCodex)

	sess, ok := s.GetById("s1")
	assert.True(t, ok)
	assert.Equal(t, Id("s1"), sess.Meta.SessionId)
}

func TestList_Empty(t *testing.T) {
	s := NewStore(10)
	assert.Empty(t, s.List())
}

func TestList_SortedByLastActive(t *testing.T) {
	s := NewStore(10)
	now := time.Now()

	s1 := s.getOrCreate("s1", AgentClaude)
	s1.LastActive = now.Add(-2 * time.Hour)

	s2 := s.getOrCreate("s2", AgentCodex)
	s2.LastActive = now

	s3 := s.getOrCreate("s3", AgentClaude)
	s3.LastActive = now.Add(-1 * time.Hour)

	list := s.List()
	assert.Len(t, list, 3)
	assert.Equal(t, Id("s2"), list[0].Meta.SessionId)
	assert.Equal(t, Id("s3"), list[1].Meta.SessionId)
	assert.Equal(t, Id("s1"), list[2].Meta.SessionId)
}

func TestList_FilteredByAgent(t *testing.T) {
	s := NewStore(10)
	now := time.Now()

	s1 := s.getOrCreate("s1", AgentClaude)
	s1.LastActive = now.Add(-2 * time.Hour)

	s2 := s.getOrCreate("s2", AgentCodex)
	s2.LastActive = now

	s3 := s.getOrCreate("s3", AgentClaude)
	s3.LastActive = now.Add(-1 * time.Hour)

	claude := s.List(AgentClaude)
	assert.Len(t, claude, 2)
	assert.Equal(t, Id("s3"), claude[0].Meta.SessionId)
	assert.Equal(t, Id("s1"), claude[1].Meta.SessionId)

	codex := s.List(AgentCodex)
	assert.Len(t, codex, 1)
	assert.Equal(t, Id("s2"), codex[0].Meta.SessionId)
}

func TestMostRecent_Empty(t *testing.T) {
	s := NewStore(10)
	_, ok := s.Last()
	assert.False(t, ok)
}

func TestMostRecent(t *testing.T) {
	s := NewStore(10)
	now := time.Now()

	s1 := s.getOrCreate("s1", AgentClaude)
	s1.LastActive = now.Add(-1 * time.Hour)

	s2 := s.getOrCreate("s2", AgentCodex)
	s2.LastActive = now

	sess, ok := s.Last()
	assert.True(t, ok)
	assert.Equal(t, Id("s2"), sess.Meta.SessionId)
}

func TestLast_FilteredByAgent(t *testing.T) {
	s := NewStore(10)
	now := time.Now()

	s1 := s.getOrCreate("s1", AgentClaude)
	s1.LastActive = now.Add(-1 * time.Hour)

	s2 := s.getOrCreate("s2", AgentCodex)
	s2.LastActive = now

	sess, ok := s.Last(AgentClaude)
	assert.True(t, ok)
	assert.Equal(t, Id("s1"), sess.Meta.SessionId)

	sess, ok = s.Last(AgentCodex)
	assert.True(t, ok)
	assert.Equal(t, Id("s2"), sess.Meta.SessionId)
}

func TestConcurrentAccess(t *testing.T) {
	s := NewStore(10)
	var wg sync.WaitGroup

	// Concurrent readers and writers. Writers only use GetOrCreate (which holds the lock).
	// This matches real usage: the watcher serializes all meta/turn writes behind its own mutex.
	for i := 0; i < 50; i++ {
		id := Id("session-" + string(rune('a'+i%10)))

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.getOrCreate(id, AgentClaude)
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
