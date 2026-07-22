# Data Integrity

Design doctrine for state-bearing changes. Planning skills (feature-design, feature-refactor) consult this
alongside `hot-items.md` when a change touches persistence, messaging, or stateful flows; reviews
(railroad-review) check changed code against it.

1. Migration history is append-only — never edit or delete an applied migration; corrections are new migrations.
2. No auto-complete or auto-rollback timers on stateful flows — state transitions fire on explicit events, never on clocks.
3. Non-idempotent side effects (sends, charges, external calls) claim before they fire: persist the claim in the same transaction, so a retry sees the claim and skips.
4. Enum handling is exhaustive — every switch over a domain enum names all members; no `default` that silently swallows new ones.
5. Dev/test credentials are deterministic and documented — never generated per run.
6. Identity resolution uses exact lookups — fuzzy matching only with explicit approval.
