# Stop Conditions

Generic stop conditions for implementation and refactor plans. The planning skills (feature-design, feature-refactor) copy these verbatim into every plan's `## Stop conditions` section, followed by their skill-specific and plan-specific ones.

1. An approved signature/contract can't hold as planned → stop and report. Never improvise architecture mid-edit.
2. Second failed fix on the same mechanism → stop, research the actual cause, redesign. No third band-aid.
3. Missing prerequisite (generated code, running infra) → run the producing step. If infrastructure is down, ask. Never skip validation, never start infrastructure yourself.
4. Discovered work materially exceeds the approved scope → ask before continuing.
5. You find the same kind of bug a second time: inside your own diff → fix every instance in the diff now. Pre-existing, outside the diff → report it and ask before searching further; sweeps eat context and are the user's call.
6. A structural obstacle (import cycle, package visibility) tempts a new abstraction (interface, DTO, wrapper) → stop and report. The fix is relocating the component, not indirection.
