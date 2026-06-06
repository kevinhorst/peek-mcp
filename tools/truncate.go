package tools

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/kevinhorst/peek-mcp/session"
)

const (
	MaxResponseBytes     = 800 * 1024 // 800KB total response budget
	DefaultTurnTextMax   = 16 * 1024  // 16KB per turn
	DefaultPlanMax       = 100 * 1024 // 100KB
	DefaultDiffMax       = 400 * 1024 // 400KB
	DefaultUncommDiffMax = 200 * 1024 // 200KB
)

// truncateString truncates s to maxBytes, appending a truncation marker.
// It backs up to the last valid UTF-8 boundary to avoid broken runes.
func truncateString(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}

	cut := maxBytes
	for cut > 0 && !utf8.ValidString(s[:cut]) {
		cut--
	}

	removed := len(s) - cut
	return s[:cut] + fmt.Sprintf("\n[truncated: %d bytes removed]", removed)
}

// truncateTurns returns a new slice of copied turns with each Text field truncated.
// Original turns in the store are not modified.
func truncateTurns(turns []*session.Turn, maxPerTurn int) []*session.Turn {
	if maxPerTurn <= 0 {
		return turns
	}

	result := make([]*session.Turn, len(turns))
	for i, t := range turns {
		if len(t.Text) <= maxPerTurn {
			result[i] = t
			continue
		}
		copied := *t
		copied.Text = truncateString(t.Text, maxPerTurn)
		result[i] = &copied
	}
	return result
}

// enforceResponseBudget trims a sessionFullResult to fit within the given byte budget.
// It trims in priority order: diff first, then plan, then oldest turns.
func enforceResponseBudget(result *sessionFullResult, budget int) *sessionFullResult {
	data, err := json.Marshal(result)
	if err != nil || len(data) <= budget {
		return result
	}

	out := &sessionFullResult{
		Turns: result.Turns,
		Plan:  result.Plan,
		Diff:  result.Diff,
	}

	// Trim diff first (independently fetchable via session_diff)
	if len(data) > budget && len(out.Diff) > 0 {
		target := len(out.Diff) - (len(data) - budget) - 128 // 128 bytes margin
		if target < 0 {
			target = 0
		}
		out.Diff = truncateString(out.Diff, target)
		data, _ = json.Marshal(out)
	}

	// Trim plan (independently fetchable via session_plan)
	if len(data) > budget && len(out.Plan) > 0 {
		target := len(out.Plan) - (len(data) - budget) - 128
		if target < 0 {
			target = 0
		}
		out.Plan = truncateString(out.Plan, target)
		data, _ = json.Marshal(out)
	}

	// Drop oldest turns one at a time
	for len(data) > budget && len(out.Turns) > 1 {
		out.Turns = out.Turns[1:]
		data, _ = json.Marshal(out)
	}

	return out
}
