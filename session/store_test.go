package session

import (
	"os"
	"path/filepath"
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

func TestAddTurn_PlanInlineContent(t *testing.T) {
	s := NewStore(10)
	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		PlanFilePath: "/nonexistent/plan.md",
		PlanContent:  "# Inline Plan",
		Meta:         &Meta{SessionId: "s1"},
	})

	sess, _ := s.GetById("s1")
	assert.Equal(t, "# Inline Plan", sess.PlanContent)
	assert.Equal(t, "/nonexistent/plan.md", sess.PlanFilePath)
}

func TestAddTurn_PlanFileReadFallback(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.md")
	os.WriteFile(planPath, []byte("# Disk Plan"), 0644)

	s := NewStore(10)
	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		PlanFilePath: planPath,
		Meta:         &Meta{SessionId: "s1"},
	})

	sess, _ := s.GetById("s1")
	assert.Equal(t, "# Disk Plan", sess.PlanContent)
}

func TestAddTurn_PlanFileReadFailure_PreservesExisting(t *testing.T) {
	s := NewStore(10)
	// First turn sets plan content via inline
	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		PlanFilePath: "/some/plan.md",
		PlanContent:  "# Existing Plan",
		Meta:         &Meta{SessionId: "s1"},
	})

	// Second turn references a non-existent file with no inline content
	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		PlanFilePath: "/nonexistent/plan.md",
		Meta:         &Meta{SessionId: "s1"},
	})

	sess, _ := s.GetById("s1")
	// PlanFilePath updated but content preserved (file read failed, no inline content)
	assert.Equal(t, "/nonexistent/plan.md", sess.PlanFilePath)
	assert.Equal(t, "# Existing Plan", sess.PlanContent)
}

func TestAddTurn_PlanWorktreeFallback(t *testing.T) {
	// Simulate worktree: plan file lives at <cwd>/.claude/plans/<name>.md
	// but the attachment reports ~/.claude/plans/<name>.md (wrong path)
	cwd := t.TempDir()
	plansDir := filepath.Join(cwd, ".claude", "plans")
	os.MkdirAll(plansDir, 0755)
	os.WriteFile(filepath.Join(plansDir, "my-plan.md"), []byte("# Worktree Plan"), 0644)

	s := NewStore(10)
	s.AddTurnBySessionId("s1", SourceClaude, &Turn{
		PlanFilePath: "/Users/someone/.claude/plans/my-plan.md", // wrong global path
		Meta:         &Meta{SessionId: "s1", CWD: cwd},
	})

	sess, _ := s.GetById("s1")
	assert.Equal(t, "# Worktree Plan", sess.PlanContent)
	assert.Equal(t, filepath.Join(plansDir, "my-plan.md"), sess.PlanFilePath)
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
