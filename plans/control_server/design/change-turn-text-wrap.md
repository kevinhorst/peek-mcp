# Turn text wrapping — Change Plan

route: `change`

## TLDR
- Turn cards render their text in `.snippet pre` with default `white-space: pre` and `overflow-x: auto` — long lines run unwrapped left to right behind a horizontal scrollbar.
- Every SSE `peek-refresh` (throttled 1s) swaps the whole `.turns-panel` via `outerHTML`, replacing the `pre` elements and resetting that horizontal scrollbar to the left edge — the bar "jumping back to the start" on each turn update.
- Fix: wrap the text — add `white-space: pre-wrap; overflow-wrap: anywhere;` to `.snippet pre`, mirroring the repo's existing wrapped-pre patterns (`.value-view`/`pre.file-view`, `.card .finding blockquote`). No horizontal scrollbar exists anymore, so there is nothing left to reset; both drivers are resolved by this one CSS change.

## Context
- Turn cards: [_turns.html:60-61](control/templates/_turns.html) render `Thinking`/`Text` inside `<div class="snippet"><pre>…</pre></div>`.
- Styling: [style.css:251](control/assets/style.css) — `.snippet pre { … overflow-x: auto; … }`, no `white-space` override → `pre` default, no wrapping.
- Refresh: [_turns.html:1](control/templates/_turns.html) — `hx-trigger="peek-refresh from:body throttle:1s"`, `hx-swap="outerHTML"`; SSE events fire it ([layout.html:72-75](control/templates/layout.html)). Element replacement discards `scrollLeft`.
- Originating plan: [change-ui-ux-refresh.md](plans/control_server/design/change-ui-ux-refresh.md) — preserved `details` open state and search inputs across swaps, but per-element scroll positions were never in scope; wrapping removes the need.

## Drivers
| ID | Observed | Wanted | Impact | Origin |
|---|---|---|---|---|
| DR1 | Horizontal scrollbar of a turn's text resets to the start on every turn update (outerHTML swap replaces the scrolled `pre`) | Reading position survives updates | behavioral | usage |
| DR2 | Turn text renders left to right as one unwrapped line, no breakpoints | Text wraps at the card width | behavioral | usage |

## Scope
- **In**
  - **`.snippet pre` wrapping:** one CSS rule change — both drivers, one cause.
- **Out**
  - **Generic scroll preservation across htmx swaps:** not needed once no per-element scrollbar exists; the window scrollbar is not reset by `outerHTML` swaps.
  - **Swap-strategy change (morph/`hx-preserve`):** heavier mechanism for the same symptom; rejected as long as wrapping resolves it.
- **Not changed**
  - **Turn card markup** ([_turns.html:52-64](control/templates/_turns.html)) — untouched.
  - **Other `pre` surfaces** (`.md-body pre`, `pre.file-view`, `.value-view`) — already wrapped or deliberately scrollable.
- **Deferred findings**
  - **Vertical shift on new turns:** turns render newest-first, so a new turn prepends above the reading position and shifts content down on refresh. Distinct mechanism (content insertion, not state reset); only relevant if jumping persists after this fix — new driver then.

## Assumptions
| Assumption | Reality | Location |
|---|---|---|
| "The Bar" = the horizontal scrollbar of the turn text | Consistent with DR2's "displays the text left to right": reading an unwrapped line requires scrolling the `pre` right; each 1s swap resets `scrollLeft` to 0 — "back to the start". The window's vertical scrollbar is not reset by `outerHTML` swaps | [style.css:251](control/assets/style.css), [_turns.html:1](control/templates/_turns.html) |

## Current state
- [style.css:251](control/assets/style.css) — `.snippet pre { background: var(--surface); border: 1px solid var(--line); border-radius: 4px; margin: 4px 0 0; overflow-x: auto; padding: 6px 10px; }` — the only unwrapped content `pre` in the UI.
- Wrapped-pre exemplars in the same file: `.value-view, pre.file-view` ([style.css:383-389](control/assets/style.css)) — `white-space: pre-wrap` with `overflow-x: auto` kept; `.card .finding blockquote` ([style.css:234-240](control/assets/style.css)) — `white-space: pre-wrap; overflow-wrap: anywhere`.

## Target state
- `.snippet pre` additionally carries `white-space: pre-wrap; overflow-wrap: anywhere;`.
- **Principle — remove the state instead of preserving it.** A scrollbar that no longer exists cannot be reset by a swap. Mechanism: CSS line wrapping (`pre-wrap` keeps whitespace/newlines, `overflow-wrap: anywhere` breaks unbroken tokens — paths, URLs — common in transcripts).

## Behavior contract
- Whitespace and newlines in turn text render exactly as before (`pre-wrap` preserves them).
- Thinking snippets inherit the change (`.snippet.thinking pre` only recolors).
- Intentional change (DR1+DR2): long lines wrap at card width; the horizontal scrollbar disappears.

## Decisions
| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| D1 | Fix the reset (preserve `scrollLeft` across swaps) or remove its cause (wrap) | Current state: only `.snippet pre` is unwrapped; exemplars wrap with `overflow-wrap: anywhere` | Wrap: `white-space: pre-wrap; overflow-wrap: anywhere;` on `.snippet pre`; keep `overflow-x: auto` as the exemplars do | Controllable and reliable: one declarative rule, no JS state tracking across swaps; scroll preservation would add swap-lifecycle code for a scrollbar the user doesn't want in the first place (DR2 wants wrapping regardless) |

## Open questions
Empty.

## Baseline (verified)
N/A — change route.

## Exemplar & reuse
N/A — change route (exemplars recorded in Current state).

## Changes
### Phase 1 — wrap turn text
[control/assets/style.css:251](control/assets/style.css:251):

```diff
-.snippet pre { background: var(--surface); border: 1px solid var(--line); border-radius: 4px; margin: 4px 0 0; overflow-x: auto; padding: 6px 10px; }
+.snippet pre { background: var(--surface); border: 1px solid var(--line); border-radius: 4px; margin: 4px 0 0; overflow-x: auto; overflow-wrap: anywhere; padding: 6px 10px; white-space: pre-wrap; }
```

ui: after-capture to be stored as `plans/control_server/design/ui/turns-wrap-after.png` at verification (pattern of the prior change plans' after captures).

## Hot items
N/A — pure CSS declaration change, no hot class touched.

## Tests
| Location.Method | Cases | Comment |
|---|---|---|
| — | — | not tested: CSS rendering, because the repo has no CSS/browser test harness; Go template tests don't see stylesheets |

## Test runbook
- turns-wrap: open the control UI on a session whose turns contain long unbroken lines; verify wrapping and absent horizontal scrollbar while SSE refreshes fire (no request files — visual check only).

## Contracts & sweeps
N/A — no cross-boundary contract touched; a stylesheet declaration consumed only by the control UI's own templates.

## Verification
- [ ] `make build` (or the repo's build target) passes — asset embedded, no template changes to break.
- [ ] Run the control server against a live/replayed session with long turn text: text wraps at card width, newlines preserved, no horizontal scrollbar on any turn card.
- [ ] During active SSE refreshes (turn updates arriving), confirm the reading position no longer snaps: nothing scrolls back to the start on swap (DR1 observed fixed with real session data).
- [ ] Thinking toggle on: thinking snippets wrap identically.
- [ ] Capture the after screenshot and store as `plans/control_server/design/ui/turns-wrap-after.png`.

## Stop conditions
| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | Stop and report. Never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | Stop, research the actual cause, redesign. No third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | Run the producing step; if infrastructure is down, ask. Never skip validation, never start infrastructure yourself |
| S4 | Discovered work materially exceeds approved scope | Ask before continuing |
| S5 | Same kind of bug a second time: in own diff → fix all in diff; pre-existing outside → report and ask | Sweeps are the user's call |
| S6 | Structural obstacle tempts a new abstraction | Stop and report; relocate the component, no indirection |
| S7 | Wrapping applied but the bar-reset symptom persists in the running UI | Stop — the driver is the vertical/content-shift mechanism (Deferred findings); new driver, don't improvise scroll-preservation code |

## Changelog
| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
