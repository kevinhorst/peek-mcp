# Codex Parser Fixes (Parity MVP) — Implementation Plan

## TLDR

- Implements the MVP items of the [agent_parity concept](plans/agent_parity/concept/concept.md) / [codex_parser_fixes](plans/agent_parity/concept/codex_parser_fixes.md): Codex plan ingestion, git-branch fix, full `session_meta` capture, sub-agent drop.
- `session_plan` for Codex returns the latest `<proposed_plan>` block; the carrying assistant message stays a chat turn (store plan-branch fall-through).
- `Meta.GitBranch` gets the real branch; commit hash, repo URL, originator, CLI version, source kind, fork/lineage move into a new `session.Origin` extension on `Meta`.
- Codex sub-agent rollouts are dropped at parse time — but the concept's drop mechanism is unsound as specified: the watcher shares ONE stateful codex parser across all rollout files, so concurrent sessions (parent + sub-agent) already cross-attribute turns. Fix: per-file parser instances via a factory in the watcher. This is the one deviation from "parity fixes live in the parsers only" — flagged in [Assumptions](#assumptions) and [D3](#d3).
- Usage semantics untouched (owned by usage_reporting concept).

## Context

- **Problem:** `session_plan` returns nothing for Codex; `Meta.GitBranch` carries a commit SHA ([codex/parser.go:69](codex/parser.go:69)); `session_meta` metadata is discarded; sub-agent rollouts leak into `session_list`.
- **Design source:** [concept.md](plans/agent_parity/concept/concept.md) + [codex_parser_fixes.md](plans/agent_parity/concept/codex_parser_fixes.md) — binding; Decisions there marked [USER].
- **Constraint:** `session.Store` and `tools/` stay agent-agnostic; parity logic lives in `codex/` (one sanctioned exception: the agent-agnostic store plan-branch fall-through, pre-approved in the concept).
- **Constraint:** the Codex usage fix is out of scope (usage_reporting concept owns it).

## Scope

- **In:**
  - **plan-ingestion:** `<proposed_plan>` extraction, sentinel plan path, store fall-through, `session_plan` description update.
  - **meta-capture:** `GitInfo.Branch`, `SessionMeta` extension, `session.Origin` struct on `session.Meta`, Claude `version` mapped into it.
  - **subagent-drop:** tolerant string-or-object `source` decode; sub-agent rollouts never create sessions.
  - **watcher-state:** per-file parser instances (required for subagent-drop and fixes pre-existing cross-session turn mis-attribution).
  - **docs:** condensed parity table in README.
- **Out:**
  - **usage:** cumulative token semantics — usage_reporting concept.
  - **titles:** Codex `thread_name` — codex_title_search concept.
  - **lineage-surfacing:** listing sub-agent sessions under parents — backlog.
  - **plan-history:** revision buffer — plan_history concept.
- **Not changed:**
  - **tools contracts:** no tool signature changes; description text only.
  - **claude parser flow:** only additive `version` parsing.
- **Deferred findings:**
  - **naming drift:** `CLIVersion`, `RepositoryURL`, `CWD` violate RULE-NAME-002 (`CliVersion`, `RepositoryUrl`, `Cwd`); repo-wide rename is churn beyond this change — not touched.
  - **files map growth:** `Watcher.files` never evicts entries (pre-existing; one small struct per rollout file).

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| Concept: "parser never sets `p.sessionId` → every subsequent line ignored by `sessionId == ""` guards" | **Falsified for concurrent sessions.** One parser instance serves ALL rollout files of the watcher; `p.sessionId` is whatever file's `session_meta` was parsed last. A sub-agent runs concurrently with its parent by definition — its lines would be attributed to the parent (and vice versa), not dropped. See [F1!](#f1), [D3](#d3) | [watcher.go:30](watcher/watcher.go:30), [start.go:93](cmd/start.go:93) |
| Concept: store plan branch "returns early, skipping AddTurn" and "`PlanContent != ""` is stored before any `os.ReadFile`" | Verified — content check precedes both `ReadFile` calls | [store.go:71-98](session/store.go:71) |
| Concept: Claude plan turns carry no text (fall-through is backward compatible) | Verified — `handleAttachment` never sets `Text` | [claude/parser.go:151-158](claude/parser.go:151) |
| Concept: `<proposed_plan>` tags line-anchored, later block is a full replacement | Verified in real rollout (2 blocks, tags on own lines, second supersedes) | ~/.codex/sessions/2026/07/11/rollout-2026-07-11T20-09-07-*.jsonl |
| Concept: `git.branch`, `originator`, `forked_from_id`, object-form `source` exist in real payloads | Verified — `branch: "develop"`, `originator: "Codex Desktop"`, 3 sub-agent rollouts with `subagent.thread_spawn` | sampled ~/.codex/sessions/2026/* |

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d1"></a>D1 | How a Codex plan reaches the store | [F5!](#f5), [F7!](#f7) | [USER] Extract last complete `<proposed_plan>` block in `handleAssistantMessage`; emit one turn carrying `Text` (full message) + `PlanContent` + sentinel `PlanFilePath` | Concept decision; single turn keeps message-as-chat-turn and plan in one signal |
| <a id="d2"></a>D2 | Plan-signal turns must not skip `AddTurn` for text-bearing turns | [F3!](#f3), [F4!](#f4) | Extract the existing plan-branch body into `Store.updatePlanContent`; after it, return only when `turn.Text == ""`, else fall through to `AddTurn` | Agent-agnostic (keyed on text, not agent); Claude plan turns have no text → behavior identical; extraction keeps RULE-NEST-001 |
| <a id="d3"></a>D3 | Concept's drop mechanism breaks under the shared parser ([F1!](#f1), [F2!](#f2)) | [F1!](#f1), [F2!](#f2) | **Deviation from "parsers only":** watcher takes a `func() Parser` factory; each watched file gets its own parser instance | Reliable: per-file state makes the `sessionId == ""` guard actually hold, and fixes the pre-existing cross-attribution of turns between any two live Codex sessions. Alternatives (path-keyed parser state, store-side drop) either change the `ParseLine` contract or violate the agent-agnostic store |
| <a id="d4"></a>D4 | Where captured metadata lives | [F12](#f12) | New `session.Origin` struct, pointer field `Meta.Origin`; one struct, all fields | [USER] one extension struct, no per-field drips; pointer expresses absence (RULE-POINTER-002); serializes into existing turn/meta JSON automatically |
| <a id="d5"></a>D5 | `source` is string-or-object | [F6!](#f6), [F13](#f13) | `codex.Source` with custom `UnmarshalJSON`: string → `Kind`; object → `Kind = "subagent"` + lineage; malformed → `Kind = "unknown"`, never an error | Malformed source must not drop a normal session (concept security note); named decode types per RULE-STRUCT-006 |
| <a id="d6"></a>D6 | Oversized extracted plans | — | 32 KB cap: byte-truncate + `slog.Warn` | Concept limit; garbage guard, exact rune boundary irrelevant |
| <a id="d7"></a>D7 | Lineage fields of dropped sessions never surface | [F6!](#f6) | Map `parent_thread_id` / `agent_nickname` into `Origin` anyway | Concept lists lineage in the extension; costs two assignments; lineage surfacing backlog needs it |
| <a id="d8"></a>D8 | Detached HEAD → empty branch | — | [USER] Empty branch stays empty; no SHA fallback | Concept decision |
| <a id="d9"></a>D9 | Existing identifiers off-style (RULE-NAME-002) | — | No renames in this change; recorded as deferred finding | Minimal diff; rename sweep is its own change |

## Baseline (verified)

Base branch: `main` (worktree `claude/agent-parity-concept-8a2280`, clean).

| ID | Fact | Needed for | Location |
|---|---|---|---|
| <a id="f1"></a>F1! | One watcher per agent holds ONE parser instance used for every `.jsonl` under the agent dir | [D3](#d3) | [watcher.go:30](watcher/watcher.go:30), [watcher.go:173](watcher/watcher.go:173), [start.go:73](cmd/start.go:73), [start.go:93](cmd/start.go:93) |
| <a id="f2"></a>F2! | `codex.Parser` keeps `sessionId` + `model` as instance state, set by the last `session_meta` / `turn_context` seen | [D3](#d3) | [codex/parser.go:20-23](codex/parser.go:20) |
| <a id="f3"></a>F3! | Store plan branch: `PlanContent != ""` returns before any `os.ReadFile`; every sub-branch returns early | [D2](#d2) | [store.go:71-98](session/store.go:71) |
| <a id="f4"></a>F4! | Claude plan-signal turns never carry `Text` | [D2](#d2) | [claude/parser.go:151-158](claude/parser.go:151) |
| <a id="f5"></a>F5! | `Turn.Validate` accepts plan-signal turns with only `Meta.SessionId` set — role/timestamp checks skipped | [D1](#d1) | [turn.go:31-36](session/turn.go:31) |
| <a id="f6"></a>F6! | Real payloads: `git = {commit_hash, branch, repository_url}`; `source` is `"vscode"`/`"mcp"` or `{"subagent":{"thread_spawn":{parent_thread_id, depth, agent_path, agent_nickname, agent_role}}}`; sub-agent rollouts also set `forked_from_id` | [D5](#d5), [D7](#d7) | sampled ~/.codex/sessions/2026/* |
| <a id="f7"></a>F7! | `<proposed_plan>` / `</proposed_plan>` on their own lines inside assistant `output_text`; 2 blocks in the sampled plan-mode rollout, prose between them | [D1](#d1) | ~/.codex/sessions/2026/07/11/rollout-2026-07-11T20-09-07-*.jsonl |
| <a id="f8"></a>F8 | Branch bug: `gitBranch = meta.Git.CommitHash` | [§4](#c4) | [codex/parser.go:67-70](codex/parser.go:67) |
| <a id="f9"></a>F9 | `HasPlan` derives from `PlanContent != "" \|\| PlanFilePath != ""` — sentinel satisfies it with no tools change | Tests | [tools.go:249](tools/tools.go:249) |
| <a id="f10"></a>F10 | Turns (incl. `Meta`) are JSON-marshaled straight into tool responses — new `Meta` fields surface without viewmodel changes | [D4](#d4) | [respond.go:35-42](tools/respond.go:35), [meta.go:3-8](session/meta.go:3) |
| <a id="f11"></a>F11 | `claude.Entry` does not parse the transcript `version` field | [§8](#c8) | [claude/entry.go:19-31](claude/entry.go:19) |
| <a id="f12"></a>F12 | `Meta.Update` merges field-wise on non-zero | [D4](#d4) | [meta.go:10-26](session/meta.go:10) |
| <a id="f13"></a>F13 | Anonymous struct types are banned (hot item 6) | [D5](#d5) | context/go/go.md RULE-STRUCT-006 |
| <a id="f14"></a>F14 | Makefile target `test` runs the suite | Verification | [Makefile:70](Makefile:70) |

Real data inspected: session_meta payloads with `git.branch` + object `source` (3 sub-agent rollouts), plan-mode rollout with 2 `<proposed_plan>` blocks.

## Exemplar & reuse

| Existing | Used for |
|---|---|
| `claude.Parser.handleAttachment` plan-signal turn shape | Codex plan-signal turn fields ([§5](#c5)) |
| `claude.Parser.handleCustomTitle` minimal-signal pattern | shape of drop/skip returns |
| `Meta.Update` non-zero merge pattern | `Origin` merge ([§1](#c1)) |
| `codex/parser_test.go` fixture-driven tests + `seededParser` | all new codex tests |
| `session/store_test.go` `provideCompleteStore` + table style | store fall-through tests |

- **Without exemplar:** `watcher/watcher_test.go` — the package has no tests yet (risk signal); test style follows `codex/parser_test.go` + go-tests.md.

## Changes

### <a id="c1"></a>1. Origin extension on session meta (modified)

location: `session/meta.go`
mirrors: `session.Meta` field/merge pattern

```go
type Meta struct {
	SessionId Id      `json:"session_id,omitempty"`
	CWD       string  `json:"cwd,omitempty"`
	GitBranch string  `json:"git_branch,omitempty"`
	Model     string  `json:"model,omitempty"`
	Origin    *Origin `json:"origin,omitempty"`
}

// Origin carries client/provenance metadata of the transcript that produced
// the session. Codex fills it from session_meta; Claude fills CliVersion.
type Origin struct {
	AgentNickname  string `json:"agent_nickname,omitempty"`
	CliVersion     string `json:"cli_version,omitempty"`
	CommitHash     string `json:"commit_hash,omitempty"`
	ForkedFromId   string `json:"forked_from_id,omitempty"`
	Originator     string `json:"originator,omitempty"`
	ParentThreadId string `json:"parent_thread_id,omitempty"`
	RepositoryUrl  string `json:"repository_url,omitempty"`
	SourceKind     string `json:"source_kind,omitempty"`
}
```

```diff
 func (m *Meta) Update(other *Meta) {
 	// ...
 	if other.Model != "" {
 		m.Model = other.Model
 	}
+
+	if other.Origin != nil {
+		m.Origin = other.Origin
+	}
 }
```

- **Merge granularity:** whole-struct replace, not field-wise — a session's origin comes from exactly one source line per agent; partial merges have no producer.

### <a id="c2"></a>2. Store plan branch falls through for text-bearing turns (modified)

location: `session/store.go`
mirrors: existing plan-branch body (moved verbatim into the helper)

```diff
 func (s *Store) AddTurnBySessionId(id Id, agent Agent, turn *Turn) {
 	// ...
 	// update only plan content
 	if turn.PlanFilePath != "" {
 		slog.Debug("Updating plan", "session", id)
 		session.PlanFilePath = turn.PlanFilePath
+		s.updatePlanContent(session, turn)
 
-		if turn.PlanContent != "" {
-			session.PlanContent = turn.PlanContent
-			return
-		}
-
-		if content, err := os.ReadFile(turn.PlanFilePath); err == nil {
-			session.PlanContent = string(content)
-			return
-		}
-
-		// Worktree fallback: Claude Code reports plan path as ~/.claude/plans/<name>
-		// but worktree sessions write to <cwd>/.claude/plans/<name>.
-		if cwd := turn.Meta.CWD; cwd != "" {
-			alt := filepath.Join(cwd, ".claude", "plans", filepath.Base(turn.PlanFilePath))
-			if content, err := os.ReadFile(alt); err == nil {
-				session.PlanFilePath = alt
-				session.PlanContent = string(content)
-				return
-			}
-		}
-
-		slog.Warn("Failed to read plan file", "path", turn.PlanFilePath)
-		return
+		// Codex plan turns are also chat turns; Claude plan signals carry no text
+		if turn.Text == "" {
+			return
+		}
 	}
 
 	// update user or assistent turn
 	session.AddTurn(turn)
```

New helper (body is the moved code, unchanged except `return` semantics stay local):

```go
func (s *Store) updatePlanContent(session *Session, turn *Turn) {
	if turn.PlanContent != "" {
		session.PlanContent = turn.PlanContent
		return
	}

	if content, err := os.ReadFile(turn.PlanFilePath); err == nil {
		session.PlanContent = string(content)
		return
	}

	// Worktree fallback: Claude Code reports plan path as ~/.claude/plans/<name>
	// but worktree sessions write to <cwd>/.claude/plans/<name>.
	if cwd := turn.Meta.CWD; cwd != "" {
		alt := filepath.Join(cwd, ".claude", "plans", filepath.Base(turn.PlanFilePath))
		if content, err := os.ReadFile(alt); err == nil {
			session.PlanFilePath = alt
			session.PlanContent = string(content)
			return
		}
	}

	slog.Warn("Failed to read plan file", "path", turn.PlanFilePath)
}
```

- **Single-caller helper justified:** inlining puts the worktree fallback at nesting level 3 — RULE-NEST-001 prescribes extraction per level.
- **Guard invariant kept:** `PlanContent != ""` still short-circuits before any `os.ReadFile` — the sentinel path never touches the filesystem (pinned by test).

### <a id="c3"></a>3. SessionMeta extension and tolerant source decode (modified + new)

location: `codex/session_meta.go`
mirrors: existing `SessionMeta` / `GitInfo` shape

```go
const (
	SourceKindSubagent = "subagent"
	SourceKindUnknown  = "unknown"
)

type GitInfo struct {
	Branch        string `json:"branch"`
	CommitHash    string `json:"commit_hash"`
	RepositoryURL string `json:"repository_url"`
}

type SessionMeta struct {
	Id           session.Id `json:"id"`
	CLIVersion   string     `json:"cli_version"`
	CWD          string     `json:"cwd"`
	ForkedFromId string     `json:"forked_from_id"`
	Git          *GitInfo   `json:"git"`
	Originator   string     `json:"originator"`
	Source       Source     `json:"source"`
}
```

`SessionMeta.Validate` unchanged. New `Source` type (full unit):

```go
// Source is string-or-object in rollouts: "vscode" / "mcp" for normal
// sessions, an object carrying subagent.thread_spawn for sub-agent rollouts.
type Source struct {
	AgentNickname  string
	Kind           string
	ParentThreadId string
}

func (s *Source) IsSubagent() bool {
	return s.Kind == SourceKindSubagent
}

// UnmarshalJSON never returns an error: a malformed source must degrade to
// "unknown", not fail the whole session_meta (a dropped normal session).
func (s *Source) UnmarshalJSON(data []byte) error {
	var kind string
	if err := json.Unmarshal(data, &kind); err == nil {
		s.Kind = kind
		return nil
	}

	var object sourceObject
	err := json.Unmarshal(data, &object)
	if err != nil || object.Subagent == nil || object.Subagent.ThreadSpawn == nil {
		s.Kind = SourceKindUnknown
		return nil
	}

	s.AgentNickname = object.Subagent.ThreadSpawn.AgentNickname
	s.Kind = SourceKindSubagent
	s.ParentThreadId = object.Subagent.ThreadSpawn.ParentThreadId
	return nil
}

type sourceObject struct {
	Subagent *sourceSubagent `json:"subagent"`
}

type sourceSubagent struct {
	ThreadSpawn *sourceThreadSpawn `json:"thread_spawn"`
}

type sourceThreadSpawn struct {
	AgentNickname  string `json:"agent_nickname"`
	ParentThreadId string `json:"parent_thread_id"`
}
```

- **Absent source key:** `UnmarshalJSON` not invoked → zero `Source{}`, `Kind` empty → `Origin.SourceKind` omitted from JSON. Not "unknown" — absent and malformed are distinguishable.
- **Ignored spawn fields:** `depth`, `agent_path`, `agent_role` are not modeled — nothing consumes them ([D7](#d7) covers only concept-listed lineage).

### <a id="c4"></a>4. Parser: branch fix, origin mapping, sub-agent drop (modified)

location: `codex/parser.go`
mirrors: `claude.Parser.handleAttachment` signal-turn shape

```diff
 func (p *Parser) handleSessionMeta(payload json.RawMessage, ts time.Time) *session.Turn {
 	var meta SessionMeta
 	// ... unmarshal + validate unchanged ...
 
+	// Sub-agent rollouts are separate helper sessions — the Codex analog of
+	// Claude's isSidechain filter. sessionId stays unset, so every later
+	// line of this file is ignored (parser state is per file, see watcher).
+	if meta.Source.IsSubagent() {
+		slog.Debug("handleSessionMeta: Dropping sub-agent rollout", "id", meta.Id)
+		return nil
+	}
+
 	p.sessionId = meta.Id
 
-	gitBranch := ""
-	if meta.Git != nil {
-		gitBranch = meta.Git.CommitHash
-	}
+	gitBranch := ""
+	origin := &session.Origin{
+		AgentNickname:  meta.Source.AgentNickname,
+		CliVersion:     meta.CLIVersion,
+		ForkedFromId:   meta.ForkedFromId,
+		Originator:     meta.Originator,
+		ParentThreadId: meta.Source.ParentThreadId,
+		SourceKind:     meta.Source.Kind,
+	}
+	if meta.Git != nil {
+		gitBranch = meta.Git.Branch
+		origin.CommitHash = meta.Git.CommitHash
+		origin.RepositoryUrl = meta.Git.RepositoryURL
+	}
 
 	return &session.Turn{
 		Role:      session.RoleAssistant,
 		Timestamp: ts,
 		Meta: &session.Meta{
 			SessionId: meta.Id,
 			CWD:       meta.CWD,
 			GitBranch: gitBranch,
+			Origin:    origin,
 		},
 	}
 }
```

- **RULE-RETURN-001 note:** the existing single-value `return &session.Turn{...}` stays as-is (single value, sanctioned).

### <a id="c5"></a>5. Parser: proposed_plan extraction (modified + new)

location: `codex/parser.go`
mirrors: `claude.Parser.handleAttachment` plan-signal fields

```diff
 func (p *Parser) handleAssistantMessage(item *ResponseItem, ts time.Time) *session.Turn {
 	text := p.extractText(item.Content, codexOutputTextType)
 	if text == "" {
 		slog.Debug("handleAssistantMessage: no output text found")
 		return nil
 	}
 
-	return &session.Turn{
+	turn := &session.Turn{
 		Role:      session.RoleAssistant,
 		Text:      text,
 		Timestamp: ts,
 		Meta: &session.Meta{
 			SessionId: p.sessionId,
 			Model:     p.model,
 		},
 	}
+
+	if plan := extractProposedPlan(text); plan != "" {
+		turn.PlanContent = plan
+		turn.PlanFilePath = PlanFilePathProposedPlan
+	}
+
+	return turn
 }
```

New constants and function (full units):

```go
const (
	// PlanFilePathProposedPlan marks Codex plans, which have no backing file.
	// The store never reads it: PlanContent is always set alongside.
	PlanFilePathProposedPlan = "codex:proposed_plan"

	proposedPlanOpenTag  = "<proposed_plan>"
	proposedPlanCloseTag = "</proposed_plan>"
	maxPlanBytes         = 32 * 1024
)

// extractProposedPlan returns the content of the last complete
// <proposed_plan> block; tags sit on their own lines per the plan-mode spec.
func extractProposedPlan(text string) string {
	lines := strings.Split(text, "\n")

	start := -1
	end := -1
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == proposedPlanOpenTag {
			start = index
			end = -1
			continue
		}

		isUnclosedBlock := start >= 0 && end < 0
		if trimmed == proposedPlanCloseTag && isUnclosedBlock {
			end = index
		}
	}
	if start < 0 || end < 0 {
		return ""
	}

	plan := strings.Join(lines[start+1:end], "\n")
	if len(plan) > maxPlanBytes {
		slog.Warn("extractProposedPlan: Plan exceeds size cap, truncating", "bytes", len(plan))
		plan = plan[:maxPlanBytes]
	}
	return plan
}
```

- **Last block wins:** a later open tag resets `start`/`end` — earlier blocks are forgotten (in-message); across messages the store overwrite handles it (existing behavior).
- **Unclosed trailing block:** not extracted (rollout lines carry complete messages; a tag without its close is garbage, not streaming).
- **Truncation:** byte slice may split a trailing rune — accepted for a garbage guard ([D6](#d6)).

### <a id="c6"></a>6. Watcher: per-file parser instances (modified)

location: `watcher/parser.go`, `watcher/watcher.go`
mirrors: existing `watchedFile` per-file state (offset)

`parser.go` — the interface goes exported so call sites can write the factory type:

```go
package watcher

import "github.com/kevinhorst/peek-mcp/session"

type Parser interface {
	ParseLine(line []byte) *session.Turn
}
```

`watcher.go`:

```diff
 type watchedFile struct {
 	offset int64
+	parser Parser
 }
 type Watcher struct {
 	agent    session.Agent
 	agentDir string
 	files    map[string]*watchedFile
 	mu       sync.Mutex
-	parser   parser
+	newParser func() Parser
 	store    *session.Store
 }
 
-func New(agent session.Agent, agentDir string, parser parser, store *session.Store) *Watcher {
+func New(agent session.Agent, agentDir string, newParser func() Parser, store *session.Store) *Watcher {
 	return &Watcher{
 		agent:    agent,
 		agentDir: agentDir,
-		parser:   parser,
+		newParser: newParser,
 		store:    store,
 		files:    make(map[string]*watchedFile),
 	}
 }
```

```diff
 func (w *Watcher) readNewLines(path string) error {
 	// ...
 	watched, ok := w.files[path]
 	if !ok {
-		watched = &watchedFile{}
+		watched = &watchedFile{parser: w.newParser()}
 	}
 	// ...
 		if len(line) > 0 {
-			turn := w.parser.ParseLine(line)
+			turn := watched.parser.ParseLine(line)
 			err = turn.Validate()
```

- **Why:** [D3](#d3) — codex parser state (`sessionId`, `model`) is per rollout file; a shared instance cross-attributes turns between concurrent sessions and defeats the sub-agent drop.
- **Claude cost:** `claude.Parser` is stateless; one zero-field instance per file is noise-free.

### <a id="c7"></a>7. Watcher wiring (modified)

location: `cmd/start.go`

```diff
 			go func() {
 				watchedDir := filepath.Join(claudeHome, claude.ProjectsDir)
-				err := watcher.New(session.AgentClaude, watchedDir, claude.NewParser(), store).Run(ctx)
+				newParser := func() watcher.Parser { return claude.NewParser() }
+				err := watcher.New(session.AgentClaude, watchedDir, newParser, store).Run(ctx)
```

```diff
 			go func() {
 				watchedDir := filepath.Join(codexHome, codex.SessionDir)
-				err := watcher.New(session.AgentCodex, watchedDir, codex.NewParser(), store).Run(ctx)
+				newParser := func() watcher.Parser { return codex.NewParser() }
+				err := watcher.New(session.AgentCodex, watchedDir, newParser, store).Run(ctx)
```

### <a id="c8"></a>8. Claude version into Origin (modified)

location: `claude/entry.go`, `claude/parser.go`

```diff
 type Entry struct {
 	// ...
 	Type              string          `json:"type"`
 	CustomTitle       string          `json:"customTitle"`
+	Version           string          `json:"version"`
 }
```

In both `handleUser` and `handleAssistant` (same three lines each; `handleAssistant` shown):

```diff
 func (p *Parser) handleAssistant(entry *Entry) *session.Turn {
 	// ...
+	var origin *session.Origin
+	if entry.Version != "" {
+		origin = &session.Origin{CliVersion: entry.Version}
+	}
+
 	turn := &session.Turn{
 		Role:      session.RoleAssistant,
 		// ...
 		Meta: &session.Meta{
 			SessionId: entry.SessionId,
 			CWD:       entry.CurrentWorkingDir,
 			GitBranch: entry.GitBranch,
 			Model:     message.Model,
+			Origin:    origin,
 		},
 	}
```

- **Nil when absent:** avoids replacing a populated `Origin` with an empty one via `Meta.Update` ([§1](#c1)).

### <a id="c9"></a>9. session_plan description (modified)

location: `tools/tools.go`

```diff
 		mcp.NewTool("session_plan",
-			mcp.WithDescription("Returns the current plan for the given session (or the most recently active session if no ID is provided). Returns an empty response if the session has no plan."),
+			mcp.WithDescription("Returns the current plan for the given session (or the most recently active session if no ID is provided). For Claude sessions this is the plan-mode plan file; for Codex the latest proposed_plan block. Returns an empty response if the session has no plan."),
```

### <a id="c10"></a>10. Fixtures (modified + new)

location: `codex/fixtures/`
mirrors: existing fixture files (real-payload-derived JSON/JSONL)

- **session_meta_full.json (modified):** add real-shaped fields — final payload:

```json
{
	"timestamp": "2026-03-29T23:45:22.019Z",
	"type": "session_meta",
	"payload": {
		"id": "sess-codex-1",
		"cwd": "/home/user/project",
		"cli_version": "1.0.0",
		"originator": "Codex Desktop",
		"source": "vscode",
		"forked_from_id": "sess-codex-0",
		"git": {
			"commit_hash": "abc123",
			"branch": "develop",
			"repository_url": "https://github.com/user/repo"
		}
	}
}
```

- **subagent_meta.json (new):** `session_meta` with object `source` (`subagent.thread_spawn` with `parent_thread_id`, `agent_nickname`) — from a real sub-agent rollout.
- **proposed_plan.jsonl (new):** `session_meta` line + two assistant messages, the second containing two `<proposed_plan>` blocks with prose between (shortened from the real 2026-07-11 rollout).

### <a id="c11a"></a>11a. session_list exposes session-level meta (modified)

location: `tools/viewmodels.go`, `tools/tools.go`

- **Why (verification finding):** F10 held per turn, but the only turn carrying `Origin`/`GitBranch` for Codex is the text-less session_meta turn, which never enters the turn buffer — session-level `Session.Meta` was captured but no tool serialized it.
- **[USER] fix:** `sessionListItem` gains `Meta session.Meta` (json `meta`); populated from `sess.Meta` in `sessionListHandler`; session_list description updated accordingly. Agent-agnostic, additive JSON.

### <a id="c11"></a>11. README parity table (modified)

location: `README.md`

- Condensed 3-column version of the concept's parity matrix (Capability | Claude | Codex), rows: title, plan, git metadata, model, usage, tool calls, sub-agents, pagination — no Status/Action column, gaps marked with the owning concept name.
- Placed as a new "Agent parity" section; content derived from the matrix in [concept.md](plans/agent_parity/concept/concept.md) after implementation.

## Hot items

Per [hot-items.md](context/general/hot-items.md):

| Class | Item | Example implementation |
|---|---|---|
| 3 — new/changed interface | `watcher.Parser` exported + factory param | full code in [§6](#c6)/[§7](#c7) |
| 5 — guard logic change | store plan-branch early-return removed for text-bearing turns | full code in [§2](#c2); `PlanContent != ""` short-circuit before `ReadFile` preserved and test-pinned |
| 5 — validation-adjacent | `Source.UnmarshalJSON` swallowing errors by design | full code in [§3](#c3); degradation target `unknown`, never a dropped session |
| 6 — anonymous structs | none — decode types are named (`sourceObject`, `sourceSubagent`, `sourceThreadSpawn`) | [§3](#c3) |

## Tests

| Location.Method | Cases | Comment |
|---|---|---|
| codex/parser_test.go `TestCodex_SessionMeta` | updated: `GitBranch == "develop"`<br>`Origin` fields populated (originator, cli_version, source kind, forked_from_id, commit_hash, repository_url) | fixture [§10](#c10) |
| codex/parser_test.go `TestCodex_SubagentDropped` | subagent_meta.json → nil turn<br>subsequent response_item lines → nil (sessionId unset) | mirrors `TestCodex_NoSessionMetaSkipped` |
| codex/session_meta_test.go `TestSource_UnmarshalJSON` | string-source<br>subagent-object<br>malformed-object (no thread_spawn)<br>non-string-non-object (number) | table-driven per RULE-TEST-*; malformed → `Kind == "unknown"`, no error |
| codex/parser_test.go `TestCodex_ProposedPlan` | plan extracted, `PlanFilePath` sentinel set<br>`Text` keeps the full message<br>last block wins across two blocks in one message<br>assistant message without block → no plan fields | fixture proposed_plan.jsonl |
| codex/parser_test.go `TestExtractProposedPlan` | no-block<br>unclosed-block<br>single-block<br>two-blocks-last-wins<br>oversized-truncated | table-driven on raw strings |
| session/store_test.go `TestAddTurnBySessionId_PlanSentinel` | sentinel path + `PlanContent` set → content stored, no file read (path does not exist on disk)<br>plan turn with `Text` → also lands in `Turns()`<br>plan turn without `Text` (Claude shape) → not a chat turn | pins the sentinel-never-read invariant and fall-through |
| session/meta_test.go `TestMeta_Update_Origin` | nil other.Origin keeps existing<br>non-nil replaces | new file or extend existing session tests |
| watcher/watcher_test.go `TestReadNewLines_PerFileParserState` | two codex rollout files in t.TempDir(), session_meta A → lines A → session_meta B → more lines A: A's turns stay on session A | new test file; drives `readNewLines` directly, no fsnotify |
| claude/parser_test.go `TestClaude_VersionOrigin` | entry with version → `Meta.Origin.CliVersion`<br>entry without version → `Origin` nil | |

- **Not tested:** fsnotify event delivery (infrastructure, exercised in live verification); README content.
- **Integration setup:** none — all tests are file/fixture-driven, no running infra.

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| `Meta` JSON gains `origin` object | Go producer; MCP tool consumers (Claude/Codex reading responses) | additive + omitempty — no consumer break; README example outputs checked |
| `Meta.GitBranch` semantic: SHA → branch name | Go; any downstream reading `git_branch` | accepted in concept (bug fix); no in-repo consumer parses it as SHA — verify with repo-wide grep for `git_branch` / `GitBranch` |
| `watcher.New` signature (parser → factory) | cmd/start.go (only caller) | grep `watcher.New(` — no other call sites; unexported `parser` interface removed, grep confirms no leftover references |
| Sentinel `codex:proposed_plan` in `PlanFilePath` | codex producer; store guard; `HasPlan` in tools | store never reads it (test-pinned); `UpdatePlanForPath` matches on real paths only — sentinel never equals a watched plan file path |
| session_plan description text | tools.go; README | update README tool description if it quotes the old text |

## Verification

- [ ] `make test` — all packages pass.
- [ ] `go vet ./...` and `gofmt -l .` (excluding vendor) — clean.
- [ ] `make build-local` — builds.
- [ ] Run `dist/peek-mcp start` against real homes; `session_list(agent:"codex")` shows branch names (e.g. `develop`), no 40-char SHAs, and no sub-agent sessions (the 3 known sub-agent rollouts absent).
- [ ] `session_plan` for the 2026-07-11 plan-mode rollout session returns the second (latest) plan block's markdown.
- [ ] `session_get` for a Codex session shows `origin` with originator/cli_version/source_kind; a Claude session shows `origin.cli_version`.
- [ ] Degenerate: a Codex session without `git` (real case: `git: null`) lists with empty branch, no crash.
- [ ] Degenerate: `session_plan` for a Codex session without a plan still returns "No plan found for this session".

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | stop and report — never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | stop, research the actual cause, redesign — no third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | run the producing step; if infrastructure is down, ask — never skip validation, never start infrastructure yourself |
| S4 | Discovered work materially exceeds the approved scope | ask before continuing |
| S5 | Same kind of bug found a second time: in own diff → fix all in diff; pre-existing outside diff | report and ask before sweeping |
| S6 | A structural obstacle tempts a new abstraction (interface, DTO, wrapper) | stop and report — relocate, don't indirect |
| S7 | The store fall-through breaks a Claude plan/title flow in existing tests | stop and report — the fall-through condition is the approved mechanism, don't patch around it |
| S8 | Real rollouts show `source` shapes beyond string / subagent-object | stop and report the new shape before extending the decode |
| S9 | Exporting `watcher.Parser` collides with another design constraint (e.g. import cycle) | stop and report — do not add an adapter layer |

## Open questions

_None._

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
| 2026-07-19 | adjust: verification finding | F10 incomplete — session-level Meta was captured but unexposed; [USER] chose session_list meta exposure (§11a) |
