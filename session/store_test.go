package session

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// provideCompleteStore returns a Store pre-populated with two sessions:
//   - "s1" (Claude, titled "Login simplification", active 1h ago)
//   - "s2" (Codex, titled "Auth refactor", active now)
func provideCompleteStore() *Store {
	s := NewStore(10)
	now := time.Now()

	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		CustomTitle: "Login simplification",
		Meta:        &Meta{SessionId: "s1"},
		TitleSource: TitleSourceCustom,
	})
	s.AddTurnBySessionId("s1", AgentClaude, &Turn{
		Role:      RoleUser,
		Text:      "What does this do?",
		Timestamp: now.Add(-1 * time.Hour),
		Meta:      &Meta{SessionId: "s1", CWD: "/project", GitBranch: "main"},
	})

	s.AddTurnBySessionId("s2", AgentCodex, &Turn{
		CustomTitle: "Auth refactor",
		Meta:        &Meta{SessionId: "s2"},
		TitleSource: TitleSourceCustom,
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

		store:   NewStore(10),
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
	s := NewStore(10)
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
	s := NewStore(10)
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
		_expectedTitle       string
		_expectedTitleSource TitleSource
		_expectedTurnCount   int
		_id                  string

		store *Store
	}

	tests := make([]*testCase, 0)

	// pass-set-title
	setTitleStore := NewStore(10)
	setTitleStore.AddTurnBySessionId("s1", AgentClaude, &Turn{
		CustomTitle: "Login simplification",
		Meta:        &Meta{SessionId: "s1"},
		TitleSource: TitleSourceCustom,
	})

	test := &testCase{
		_id:                  "pass-set-title",
		_expectedTitle:       "Login simplification",
		_expectedTitleSource: TitleSourceCustom,

		store: setTitleStore,
	}
	tests = append(tests, test)

	// pass-update-title
	updateTitleStore := NewStore(10)
	updateTitleStore.AddTurnBySessionId("s1", AgentClaude, &Turn{
		CustomTitle: "Old title",
		Meta:        &Meta{SessionId: "s1"},
		TitleSource: TitleSourceCustom,
	})
	updateTitleStore.AddTurnBySessionId("s1", AgentClaude, &Turn{
		CustomTitle: "New title",
		Meta:        &Meta{SessionId: "s1"},
		TitleSource: TitleSourceCustom,
	})

	test = &testCase{
		_id:                  "pass-update-title",
		_expectedTitle:       "New title",
		_expectedTitleSource: TitleSourceCustom,

		store: updateTitleStore,
	}
	tests = append(tests, test)

	// pass-index-source-set
	indexTitleStore := NewStore(10)
	indexTitleStore.AddTurnBySessionId("s1", AgentCodex, &Turn{
		CustomTitle: "Propagate Supabase schema",
		Meta:        &Meta{SessionId: "s1"},
		TitleSource: TitleSourceIndex,
	})

	test = &testCase{
		_id:                  "pass-index-source-set",
		_expectedTitle:       "Propagate Supabase schema",
		_expectedTitleSource: TitleSourceIndex,

		store: indexTitleStore,
	}
	tests = append(tests, test)

	// pass-repeated-title-does-not-add-turn
	repeatedTitleStore := NewStore(10)
	repeatedTitleStore.AddTurnBySessionId("s1", AgentCodex, &Turn{
		CustomTitle: "Propagate Supabase schema",
		Meta:        &Meta{SessionId: "s1"},
		TitleSource: TitleSourceIndex,
	})
	repeatedTitleStore.AddTurnBySessionId("s1", AgentCodex, &Turn{
		CustomTitle: "Propagate Supabase schema",
		Meta:        &Meta{SessionId: "s1"},
		TitleSource: TitleSourceIndex,
	})

	test = &testCase{
		_id:                  "pass-repeated-title-does-not-add-turn",
		_expectedTitle:       "Propagate Supabase schema",
		_expectedTitleSource: TitleSourceIndex,
		_expectedTurnCount:   0,

		store: repeatedTitleStore,
	}
	tests = append(tests, test)

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			sess, ok := test.store.GetById("s1")
			assert.True(t, ok, "session should exist")
			assert.Equal(t, test._expectedTitle, sess.Title)
			assert.Equal(t, test._expectedTitleSource, sess.TitleSource)
			assert.Len(t, sess.Turns(10), test._expectedTurnCount)
		})
	}
}

func provideTitledSession(s *Store, id Id, agent Agent, title string, lastActive time.Time) {
	s.AddTurnBySessionId(id, agent, &Turn{
		CustomTitle: title,
		Meta:        &Meta{SessionId: id},
		TitleSource: TitleSourceIndex,
		Timestamp:   lastActive,
	})
}

func TestGetByTitle(t *testing.T) {
	type testCase struct {
		_expectedErrContains string
		_expectedSessionId   Id
		_id                  string
		_shouldPass          bool

		agent Agent
		query string
		store *Store
	}

	now := time.Now()
	tests := make([]*testCase, 0)

	// pass-exact-match
	test := &testCase{
		_id:                "pass-exact-match",
		_shouldPass:        true,
		_expectedSessionId: "s1",

		query: "Login simplification",
		store: provideCompleteStore(),
	}
	tests = append(tests, test)

	// pass-case-insensitive
	test = &testCase{
		_id:                "pass-case-insensitive",
		_shouldPass:        true,
		_expectedSessionId: "s1",

		query: "login simplification",
		store: provideCompleteStore(),
	}
	tests = append(tests, test)

	// pass-substring-match
	test = &testCase{
		_id:                "pass-substring-match",
		_shouldPass:        true,
		_expectedSessionId: "s1",

		query: "Login",
		store: provideCompleteStore(),
	}
	tests = append(tests, test)

	// fail-not-found
	test = &testCase{
		_id:                  "fail-not-found",
		_shouldPass:          false,
		_expectedErrContains: "no session matching title",

		query: "nonexistent",
		store: provideCompleteStore(),
	}
	tests = append(tests, test)

	// fail-substring-ambiguous-lists-candidates
	ambiguousStore := NewStore(10)
	provideTitledSession(ambiguousStore, "a1", AgentCodex, "Propagate Supabase schema", now.Add(-1*time.Hour))
	provideTitledSession(ambiguousStore, "a2", AgentCodex, "Trim schema from supabase types", now)

	test = &testCase{
		_id:                  "fail-substring-ambiguous-lists-candidates",
		_shouldPass:          false,
		_expectedErrContains: "multiple sessions match title",

		query: "supabase",
		store: ambiguousStore,
	}
	tests = append(tests, test)

	// pass-exact-duplicate-most-recent-wins
	duplicateStore := NewStore(10)
	provideTitledSession(duplicateStore, "d1", AgentCodex, "Propagate Supabase schema", now.Add(-1*time.Hour))
	provideTitledSession(duplicateStore, "d2", AgentCodex, "Propagate Supabase schema", now)

	test = &testCase{
		_id:                "pass-exact-duplicate-most-recent-wins",
		_shouldPass:        true,
		_expectedSessionId: "d2",

		query: "Propagate Supabase schema",
		store: duplicateStore,
	}
	tests = append(tests, test)

	// pass-agent-filtered
	crossAgentStore := NewStore(10)
	provideTitledSession(crossAgentStore, "c1", AgentClaude, "Auth refactor", now)
	provideTitledSession(crossAgentStore, "c2", AgentCodex, "Auth refactor", now.Add(-1*time.Hour))

	test = &testCase{
		_id:                "pass-agent-filtered",
		_shouldPass:        true,
		_expectedSessionId: "c2",

		agent: AgentCodex,
		query: "Auth refactor",
		store: crossAgentStore,
	}
	tests = append(tests, test)

	// fail-agent-mismatch
	test = &testCase{
		_id:                  "fail-agent-mismatch",
		_shouldPass:          false,
		_expectedErrContains: "no session matching title",

		agent: AgentClaude,
		query: "Auth refactor",
		store: provideCompleteStore(),
	}
	tests = append(tests, test)

	// fail-invalid-agent-matches-nothing
	test = &testCase{
		_id:                  "fail-invalid-agent-matches-nothing",
		_shouldPass:          false,
		_expectedErrContains: "no session matching title",

		agent: "bogus",
		query: "Login simplification",
		store: provideCompleteStore(),
	}
	tests = append(tests, test)

	// fail-title-update-removes-old-index
	storeWithUpdate := NewStore(10)
	provideTitledSession(storeWithUpdate, "s1", AgentClaude, "Old title", now.Add(-1*time.Hour))
	provideTitledSession(storeWithUpdate, "s1", AgentClaude, "New title", now)

	test = &testCase{
		_id:                  "fail-title-update-removes-old-index",
		_shouldPass:          false,
		_expectedErrContains: "no session matching title",

		query: "Old title",
		store: storeWithUpdate,
	}
	tests = append(tests, test)

	// pass-title-update-new-title-resolves
	test = &testCase{
		_id:                "pass-title-update-new-title-resolves",
		_shouldPass:        true,
		_expectedSessionId: "s1",

		query: "New title",
		store: storeWithUpdate,
	}
	tests = append(tests, test)

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			sess, err := test.store.GetByTitle(test.query, test.agent)
			assert.Equalf(t, test._shouldPass, err == nil, "err = %v", err)
			if test._shouldPass {
				assert.Equal(t, test._expectedSessionId, sess.Meta.SessionId)
				return
			}
			assert.ErrorContains(t, err, test._expectedErrContains)
		})
	}
}

func TestGetByTitle_AmbiguityCandidates(t *testing.T) {
	now := time.Now()
	s := NewStore(10)
	for i := range 7 {
		id := Id("m" + string(rune('0'+i)))
		provideTitledSession(s, id, AgentCodex, "Propagate Supabase schema "+string(rune('0'+i)), now.Add(-time.Duration(i)*time.Hour))
	}

	_, err := s.GetByTitle("supabase", AgentCodex)
	assert.ErrorContains(t, err, "multiple sessions match title")
	assert.ErrorContains(t, err, "m0")
	assert.ErrorContains(t, err, "m4")
	assert.NotContains(t, err.Error(), "m5")
	assert.NotContains(t, err.Error(), "m6")
}

func TestAddTurn_TitlePrecedence(t *testing.T) {
	type testCase struct {
		_expectedTitle       string
		_expectedTitleSource TitleSource
		_id                  string

		store *Store
	}

	now := time.Now()

	provideUserTurn := func(text string) *Turn {
		return &Turn{
			Role:      RoleUser,
			Text:      text,
			Timestamp: now,
			Meta:      &Meta{SessionId: "s1"},
		}
	}
	provideTitleTurn := func(title string, source TitleSource) *Turn {
		return &Turn{
			CustomTitle: title,
			Meta:        &Meta{SessionId: "s1"},
			TitleSource: source,
		}
	}

	tests := make([]*testCase, 0)

	// derived-then-index-overwrites
	derivedThenIndex := NewStore(10)
	derivedThenIndex.AddTurnBySessionId("s1", AgentCodex, provideUserTurn("Fix the login flow"))
	derivedThenIndex.AddTurnBySessionId("s1", AgentCodex, provideTitleTurn("Login fix", TitleSourceIndex))

	test := &testCase{
		_id:                  "derived-then-index-overwrites",
		_expectedTitle:       "Login fix",
		_expectedTitleSource: TitleSourceIndex,

		store: derivedThenIndex,
	}
	tests = append(tests, test)

	// index-then-derived-ignored
	indexThenDerived := NewStore(10)
	indexThenDerived.AddTurnBySessionId("s1", AgentCodex, provideTitleTurn("Login fix", TitleSourceIndex))
	indexThenDerived.AddTurnBySessionId("s1", AgentCodex, provideUserTurn("Fix the login flow"))

	test = &testCase{
		_id:                  "index-then-derived-ignored",
		_expectedTitle:       "Login fix",
		_expectedTitleSource: TitleSourceIndex,

		store: indexThenDerived,
	}
	tests = append(tests, test)

	// index-then-custom-overwrites
	indexThenCustom := NewStore(10)
	indexThenCustom.AddTurnBySessionId("s1", AgentCodex, provideTitleTurn("Login fix", TitleSourceIndex))
	indexThenCustom.AddTurnBySessionId("s1", AgentCodex, provideTitleTurn("My login fix", TitleSourceCustom))

	test = &testCase{
		_id:                  "index-then-custom-overwrites",
		_expectedTitle:       "My login fix",
		_expectedTitleSource: TitleSourceCustom,

		store: indexThenCustom,
	}
	tests = append(tests, test)

	// custom-then-index-ignored
	customThenIndex := NewStore(10)
	customThenIndex.AddTurnBySessionId("s1", AgentCodex, provideTitleTurn("My login fix", TitleSourceCustom))
	customThenIndex.AddTurnBySessionId("s1", AgentCodex, provideTitleTurn("Login fix", TitleSourceIndex))

	test = &testCase{
		_id:                  "custom-then-index-ignored",
		_expectedTitle:       "My login fix",
		_expectedTitleSource: TitleSourceCustom,

		store: customThenIndex,
	}
	tests = append(tests, test)

	// index-rename-same-rank-overwrites
	indexRename := NewStore(10)
	indexRename.AddTurnBySessionId("s1", AgentCodex, provideTitleTurn("Login fix", TitleSourceIndex))
	indexRename.AddTurnBySessionId("s1", AgentCodex, provideTitleTurn("Login rework", TitleSourceIndex))

	test = &testCase{
		_id:                  "index-rename-same-rank-overwrites",
		_expectedTitle:       "Login rework",
		_expectedTitleSource: TitleSourceIndex,

		store: indexRename,
	}
	tests = append(tests, test)

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			sess, ok := test.store.GetById("s1")
			assert.True(t, ok, "session should exist")
			assert.Equal(t, test._expectedTitle, sess.Title)
			assert.Equal(t, test._expectedTitleSource, sess.TitleSource)
		})
	}
}

func TestAddTurn_DerivedTitle(t *testing.T) {
	type testCase struct {
		_expectedTitle       string
		_expectedTitleSource TitleSource
		_id                  string

		texts []string
	}

	tests := make([]*testCase, 0)

	// derives-first-line
	test := &testCase{
		_id:                  "derives-first-line",
		_expectedTitle:       "Fix the login flow",
		_expectedTitleSource: TitleSourceDerived,

		texts: []string{"Fix the login flow\nIt breaks on empty passwords."},
	}
	tests = append(tests, test)

	// truncates-80-runes
	test = &testCase{
		_id:                  "truncates-80-runes",
		_expectedTitle:       strings.Repeat("ü", 80),
		_expectedTitleSource: TitleSourceDerived,

		texts: []string{strings.Repeat("ü", 100)},
	}
	tests = append(tests, test)

	// skips-wrapper-prefixes
	test = &testCase{
		_id:            "skips-wrapper-prefixes",
		_expectedTitle: "",

		texts: []string{"<environment_context>\n<current_date>2026-07-19</current_date>"},
	}
	tests = append(tests, test)

	// skips-agents-md-wrapper
	test = &testCase{
		_id:            "skips-agents-md-wrapper",
		_expectedTitle: "",

		texts: []string{"# AGENTS.md instructions\n\n<INSTRUCTIONS>never praise</INSTRUCTIONS>"},
	}
	tests = append(tests, test)

	// derives-from-next-user-turn-after-wrapper
	test = &testCase{
		_id:                  "derives-from-next-user-turn-after-wrapper",
		_expectedTitle:       "Fix the login flow",
		_expectedTitleSource: TitleSourceDerived,

		texts: []string{"<recommended_plugins>plugin list</recommended_plugins>", "Fix the login flow"},
	}
	tests = append(tests, test)

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			s := NewStore(10)
			for i, text := range test.texts {
				s.AddTurnBySessionId("s1", AgentCodex, &Turn{
					Role:      RoleUser,
					Text:      text,
					Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
					Meta:      &Meta{SessionId: "s1"},
				})
			}

			sess, ok := s.GetById("s1")
			assert.True(t, ok, "session should exist")
			assert.Equal(t, test._expectedTitle, sess.Title)
			assert.Equal(t, test._expectedTitleSource, sess.TitleSource)
		})
	}
}

func TestAddTurn_AssistantTurnDoesNotDerive(t *testing.T) {
	s := NewStore(10)
	s.AddTurnBySessionId("s1", AgentCodex, &Turn{
		Role:      RoleAssistant,
		Text:      "I will fix the login flow.",
		Timestamp: time.Now(),
		Meta:      &Meta{SessionId: "s1"},
	})

	sess, _ := s.GetById("s1")
	assert.Equal(t, "", sess.Title)
	assert.Equal(t, TitleSource(""), sess.TitleSource)
}

func TestAddTurn_TitleOnlySession(t *testing.T) {
	indexTime := time.Date(2026, 4, 19, 14, 10, 36, 0, time.UTC)
	s := NewStore(10)
	s.AddTurnBySessionId("s1", AgentCodex, &Turn{
		CustomTitle: "Propagate Supabase schema",
		Meta:        &Meta{SessionId: "s1"},
		Timestamp:   indexTime,
		TitleSource: TitleSourceIndex,
	})

	// seeds-last-active-from-timestamp
	sess, ok := s.GetById("s1")
	assert.True(t, ok)
	assert.Equal(t, indexTime, sess.LastActive)

	// listed-in-session-list
	assert.Len(t, s.List(AgentCodex), 1)

	// turn-timestamp-takes-over
	turnTime := indexTime.Add(time.Hour)
	s.AddTurnBySessionId("s1", AgentCodex, &Turn{
		Role:      RoleUser,
		Text:      "Continue the migration",
		Timestamp: turnTime,
		Meta:      &Meta{SessionId: "s1"},
	})
	assert.Equal(t, turnTime, sess.LastActive)
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
