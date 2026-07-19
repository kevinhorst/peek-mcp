package session

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/stretchr/testify/assert"
)

// provideCompleteStore returns a Store pre-populated with two sessions:
//   - "s1" (Claude, titled "Login simplification", active 1h ago)
//   - "s2" (Codex, titled "Auth refactor", active now)
func provideCompleteStore() *Store {
	s := NewStore(10, events.NewBroker())
	now := time.Now()

	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		CustomTitle: "Login simplification",
		Meta:    &Meta{SessionId: "s1"},
	})
	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		Role:      RoleUser,
		Text:      "What does this do?",
		Timestamp: now.Add(-1 * time.Hour),
		Meta:      &Meta{SessionId: "s1", CWD: "/project", GitBranch: "main"},
	})

	s.AddTurnBySessionId("s2", AgentCodex, &Turn{
		CustomTitle: "Auth refactor",
		Meta:    &Meta{SessionId: "s2"},
	})
	s.AddTurnBySessionId("s2", AgentCodex, &Turn{
		Role:      RoleUser,
		Text:      "Refactor auth",
		Timestamp: now,
		Meta:      &Meta{SessionId: "s2", CWD: "/project", GitBranch: "feat"},
	})

	return s
}

func TestGetOrCreate_New(t *testing.T) {
	s := NewStore(10, events.NewBroker())
	sess := s.getOrCreate("s1", AgentClaude)

	assert.Equal(t, Id("s1"), sess.Meta.SessionId)
	assert.Equal(t, AgentClaude, sess.Agent)
	assert.NotNil(t, sess.TurnsFinished)
}

func TestGetOrCreate_Existing(t *testing.T) {
	s := NewStore(10, events.NewBroker())
	s1 := s.getOrCreate("s1", AgentClaude)
	s2 := s.getOrCreate("s1", AgentClaude)

	assert.Same(t, s1, s2)
}

func TestGetById(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		store   *Store
		queryId Id
	}

	tests := make([]*testCase, 0)

	// pass-found
	test := &testCase{
		_id:         "pass-found",
		_shouldPass: true,

		store:   provideCompleteStore(),
		queryId: "s1",
	}
	tests = append(tests, test)

	// fail-not-found
	test = &testCase{
		_id:         "fail-not-found",
		_shouldPass: false,

		store:   provideCompleteStore(),
		queryId: "nonexistent",
	}
	tests = append(tests, test)

	// fail-empty-store
	test = &testCase{
		_id:         "fail-empty-store",
		_shouldPass: false,

		store:   NewStore(10, events.NewBroker()),
		queryId: "s1",
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			sess, ok := test.store.GetById(test.queryId)
			assert.Equalf(t, test._shouldPass, ok, "GetById(%q)", test.queryId)
			if test._shouldPass {
				assert.Equal(t, test.queryId, sess.Meta.SessionId)
			}
		})
	}
}

func TestList_Empty(t *testing.T) {
	s := NewStore(10, events.NewBroker())
	assert.Empty(t, s.List())
}

func TestList_SortedByLastActive(t *testing.T) {
	s := provideCompleteStore()

	list := s.List()
	assert.Len(t, list, 2)
	assert.Equal(t, Id("s2"), list[0].Meta.SessionId)
	assert.Equal(t, Id("s1"), list[1].Meta.SessionId)
}

func TestList_FilteredByAgent(t *testing.T) {
	s := provideCompleteStore()

	claude := s.List(AgentClaude)
	assert.Len(t, claude, 1)
	assert.Equal(t, Id("s1"), claude[0].Meta.SessionId)

	codex := s.List(AgentCodex)
	assert.Len(t, codex, 1)
	assert.Equal(t, Id("s2"), codex[0].Meta.SessionId)
}

func TestMostRecent_Empty(t *testing.T) {
	s := NewStore(10, events.NewBroker())
	_, ok := s.Last()
	assert.False(t, ok)
}

func TestMostRecent(t *testing.T) {
	s := provideCompleteStore()

	sess, ok := s.Last()
	assert.True(t, ok)
	assert.Equal(t, Id("s2"), sess.Meta.SessionId)
}

func TestLast_FilteredByAgent(t *testing.T) {
	s := provideCompleteStore()

	sess, ok := s.Last(AgentClaude)
	assert.True(t, ok)
	assert.Equal(t, Id("s1"), sess.Meta.SessionId)

	sess, ok = s.Last(AgentCodex)
	assert.True(t, ok)
	assert.Equal(t, Id("s2"), sess.Meta.SessionId)
}

func TestAddTurn_PlanInlineContent(t *testing.T) {
	s := NewStore(10, events.NewBroker())
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

	s := NewStore(10, events.NewBroker())
	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		PlanFilePath: planPath,
		Meta:         &Meta{SessionId: "s1"},
	})

	sess, _ := s.GetById("s1")
	assert.Equal(t, "# Disk Plan", sess.PlanContent)
}

func TestAddTurn_PlanFileReadFailure_PreservesExisting(t *testing.T) {
	s := NewStore(10, events.NewBroker())
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

	s := NewStore(10, events.NewBroker())
	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		PlanFilePath: "/Users/someone/.claude/plans/my-plan.md", // wrong global path
		Meta:         &Meta{SessionId: "s1", CWD: cwd},
	})

	sess, _ := s.GetById("s1")
	assert.Equal(t, "# Worktree Plan", sess.PlanContent)
	assert.Equal(t, filepath.Join(plansDir, "my-plan.md"), sess.PlanFilePath)
}

func TestAddTurn_CustomTitle(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		store     *Store
		wantTitle string
	}

	tests := make([]*testCase, 0)

	// pass-set-title
	setTitleStore := NewStore(10, events.NewBroker())
	setTitleStore.AddTurnBySessionId("s1", AgentClaude, &Turn{
		CustomTitle: "Login simplification",
		Meta:    &Meta{SessionId: "s1"},
	})

	test := &testCase{
		_id:         "pass-set-title",
		_shouldPass: true,

		store:     setTitleStore,
		wantTitle: "Login simplification",
	}
	tests = append(tests, test)

	// pass-update-title
	updateTitleStore := NewStore(10, events.NewBroker())
	updateTitleStore.AddTurnBySessionId("s1", AgentClaude, &Turn{
		CustomTitle: "Old title",
		Meta:    &Meta{SessionId: "s1"},
	})
	updateTitleStore.AddTurnBySessionId("s1", AgentClaude, &Turn{
		CustomTitle: "New title",
		Meta:    &Meta{SessionId: "s1"},
	})

	test = &testCase{
		_id:         "pass-update-title",
		_shouldPass: true,

		store:     updateTitleStore,
		wantTitle: "New title",
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			sess, ok := test.store.GetById("s1")
			assert.Equalf(t, test._shouldPass, ok, "session should exist")
			assert.Equal(t, test.wantTitle, sess.Title)
		})
	}
}

func TestGetByTitle(t *testing.T) {
	type testCase struct {
		_id                string
		_shouldPass        bool
		_expectedSessionId Id

		store *Store
		query string
	}

	tests := make([]*testCase, 0)

	// pass-exact-match
	test := &testCase{
		_id:         "pass-exact-match",
		_shouldPass: true,

		store:              provideCompleteStore(),
		query:              "Login simplification",
		_expectedSessionId: "s1",
	}
	tests = append(tests, test)

	// pass-case-insensitive
	test = &testCase{
		_id:         "pass-case-insensitive",
		_shouldPass: true,

		store:              provideCompleteStore(),
		query:              "login simplification",
		_expectedSessionId: "s1",
	}
	tests = append(tests, test)

	// fail-not-found
	test = &testCase{
		_id:         "fail-not-found",
		_shouldPass: false,

		store: provideCompleteStore(),
		query: "nonexistent",
	}
	tests = append(tests, test)

	// fail-substring-does-not-match
	test = &testCase{
		_id:         "fail-substring-does-not-match",
		_shouldPass: false,

		store: provideCompleteStore(),
		query: "Login",
	}
	tests = append(tests, test)

	// fail-title-update-removes-old-index
	storeWithUpdate := NewStore(10, events.NewBroker())
	storeWithUpdate.AddTurnBySessionId("s1", AgentClaude, &Turn{
		CustomTitle: "Old title",
		Meta:    &Meta{SessionId: "s1"},
	})
	storeWithUpdate.AddTurnBySessionId("s1", AgentClaude, &Turn{
		CustomTitle: "New title",
		Meta:    &Meta{SessionId: "s1"},
	})

	test = &testCase{
		_id:         "fail-title-update-removes-old-index",
		_shouldPass: false,

		store: storeWithUpdate,
		query: "Old title",
	}
	tests = append(tests, test)

	// pass-title-update-new-title-resolves
	test = &testCase{
		_id:         "pass-title-update-new-title-resolves",
		_shouldPass: true,

		store:              storeWithUpdate,
		query:              "New title",
		_expectedSessionId: "s1",
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			sess, ok := test.store.GetByTitle(test.query)
			assert.Equalf(t, test._shouldPass, ok, "GetByTitle(%q)", test.query)
			if test._shouldPass {
				assert.Equal(t, test._expectedSessionId, sess.Meta.SessionId)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := provideCompleteStore()
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

func drainTypes(ch <-chan events.Event) []events.Type {
	types := make([]events.Type, 0)
	for {
		select {
		case ev := <-ch:
			types = append(types, ev.Type)
		default:
			return types
		}
	}
}

func TestPublish_TurnAdded(t *testing.T) {
	broker := events.NewBroker()
	ch, cancel := broker.Subscribe()
	defer cancel()
	s := NewStore(10, broker)

	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		Role:      RoleUser,
		Text:      "hello",
		Timestamp: time.Now(),
		Meta:      &Meta{SessionId: "s1"},
	})

	assert.Equal(t, []events.Type{events.TypeSessionCreated, events.TypeTurnAdded}, drainTypes(ch))
}

func TestPublish_PlanSignal(t *testing.T) {
	broker := events.NewBroker()
	ch, cancel := broker.Subscribe()
	defer cancel()
	s := NewStore(10, broker)

	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		PlanFilePath: "/nonexistent/plan.md",
		PlanContent:  "# Plan",
		Meta:         &Meta{SessionId: "s1"},
	})

	assert.Equal(t, []events.Type{events.TypeSessionCreated, events.TypePlanUpdated}, drainTypes(ch))
}

func TestPublish_UpdateDiff(t *testing.T) {
	broker := events.NewBroker()
	s := NewStore(10, broker)
	s.getOrCreate("s1", AgentClaude)

	ch, cancel := broker.Subscribe()
	defer cancel()

	s.UpdateDiff("s1", "main", "diff")
	assert.Equal(t, []events.Type{events.TypeDiffUpdated}, drainTypes(ch))

	s.UpdateDiff("unknown", "main", "diff")
	assert.Empty(t, drainTypes(ch))
}

func TestPublish_UpdateUncommittedDiff(t *testing.T) {
	broker := events.NewBroker()
	s := NewStore(10, broker)
	s.getOrCreate("s1", AgentClaude)

	ch, cancel := broker.Subscribe()
	defer cancel()

	s.UpdateUncommittedDiff("s1", "diff")
	assert.Equal(t, []events.Type{events.TypeUncommittedDiffUpdated}, drainTypes(ch))
}

func TestPublish_UpdatePlanForPath(t *testing.T) {
	broker := events.NewBroker()
	s := NewStore(10, broker)
	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		PlanFilePath: "/plans/a.md",
		PlanContent:  "# A",
		Meta:         &Meta{SessionId: "s1"},
	})
	s.getOrCreate("s2", AgentCodex)

	ch, cancel := broker.Subscribe()
	defer cancel()

	s.UpdatePlanForPath("/plans/a.md", "# A v2")

	types := drainTypes(ch)
	assert.Equal(t, []events.Type{events.TypePlanUpdated}, types)
}

func TestWithSessions(t *testing.T) {
	s := provideCompleteStore()

	var ids []Id
	s.WithSessions(nil, func(sessions []*Session) {
		for _, sess := range sessions {
			ids = append(ids, sess.Meta.SessionId)
		}
	})
	assert.Equal(t, []Id{"s2", "s1"}, ids)

	ids = nil
	s.WithSessions([]Agent{AgentClaude}, func(sessions []*Session) {
		for _, sess := range sessions {
			ids = append(ids, sess.Meta.SessionId)
		}
	})
	assert.Equal(t, []Id{"s1"}, ids)
}

func TestWithSession(t *testing.T) {
	s := provideCompleteStore()

	var title string
	found := s.WithSession("s1", func(sess *Session) {
		title = sess.Title
	})
	assert.True(t, found)
	assert.Equal(t, "Login simplification", title)

	called := false
	found = s.WithSession("unknown", func(sess *Session) {
		called = true
	})
	assert.False(t, found)
	assert.False(t, called)
}
