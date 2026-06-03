package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

func TestTruncateString_UnderLimit(t *testing.T) {
	s := "hello world"
	result := truncateString(s, 100)
	if result != s {
		t.Errorf("expected no truncation, got %q", result)
	}
}

func TestTruncateString_OverLimit(t *testing.T) {
	s := strings.Repeat("a", 1000)
	result := truncateString(s, 100)
	if len(result) > 200 { // 100 bytes + truncation marker
		t.Errorf("expected truncated result, got length %d", len(result))
	}
	if !strings.Contains(result, "[truncated:") {
		t.Error("expected truncation marker")
	}
}

func TestTruncateString_ZeroMax(t *testing.T) {
	s := "hello"
	result := truncateString(s, 0)
	if result != s {
		t.Errorf("expected no truncation for maxBytes=0, got %q", result)
	}
}

func TestTruncateString_UTF8Boundary(t *testing.T) {
	// "é" is 2 bytes in UTF-8
	s := "café" // c(1) a(1) f(1) é(2) = 5 bytes
	result := truncateString(s, 4)
	// Should cut before the é, not in the middle of it
	if strings.Contains(result, "\xc3") && !strings.Contains(result, "é") {
		t.Errorf("truncation broke a UTF-8 rune: %q", result)
	}
	if !strings.Contains(result, "[truncated:") {
		t.Error("expected truncation marker")
	}
}

func TestTruncateTurns_NoMutation(t *testing.T) {
	original := "this is a long text that should be truncated"
	turn := &session.Turn{
		Role:      session.RoleAssistant,
		Text:      original,
		Timestamp: time.Now(),
		Meta:      &session.Meta{SessionId: "test"},
	}

	result := truncateTurns([]*session.Turn{turn}, 10)

	if turn.Text != original {
		t.Errorf("original turn was mutated: %q", turn.Text)
	}
	if result[0].Text == original {
		t.Error("expected turn text to be truncated")
	}
}

func TestTruncateTurns_ShortTextUnchanged(t *testing.T) {
	turn := &session.Turn{
		Role:      session.RoleUser,
		Text:      "short",
		Timestamp: time.Now(),
		Meta:      &session.Meta{SessionId: "test"},
	}

	result := truncateTurns([]*session.Turn{turn}, 1000)

	if result[0] != turn {
		t.Error("short turn should be returned as-is (same pointer)")
	}
}

func TestEnforceResponseBudget_UnderBudget(t *testing.T) {
	result := &sessionFullResult{
		Turns: []*session.Turn{{
			Role:      session.RoleUser,
			Text:      "hello",
			Timestamp: time.Now(),
			Meta:      &session.Meta{SessionId: "test"},
		}},
		Plan: "small plan",
		Diff: "small diff",
	}

	out := enforceResponseBudget(result, MaxResponseBytes)
	if out.Truncated {
		t.Error("should not be truncated when under budget")
	}
}

func TestEnforceResponseBudget_OversizedDiff(t *testing.T) {
	result := &sessionFullResult{
		Turns: []*session.Turn{{
			Role:      session.RoleUser,
			Text:      "hello",
			Timestamp: time.Now(),
			Meta:      &session.Meta{SessionId: "test"},
		}},
		Plan: "small plan",
		Diff: strings.Repeat("x", 900*1024), // 900KB diff
	}

	out := enforceResponseBudget(result, MaxResponseBytes)
	if !out.Truncated {
		t.Error("should be marked as truncated")
	}
	if len(out.Diff) >= 900*1024 {
		t.Error("diff should have been trimmed")
	}
	if out.Plan != "small plan" {
		t.Error("plan should not have been trimmed")
	}
}
