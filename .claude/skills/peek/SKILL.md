---
name: peek
description: >
  Show the latest Claude Code session. Call when user types /peek (with optional
  count), asks what Claude Code is doing, or wants recent session turns, plan, or diff.
---

Call session_full with n from the user (default 5).

Show turns as Human/Assistant blocks. If plan is non-empty, show it under "Plan". If diff is non-empty, show it under "Diff".
