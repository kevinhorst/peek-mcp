package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func timedTurn(role Role, timestamp time.Time) *Turn {
	return &Turn{
		Role:      role,
		Text:      "text",
		Timestamp: timestamp,
		Meta:      &Meta{SessionId: "id"},
	}
}

func TestSession_AddTurnTime(t *testing.T) {
	base := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)

	// started-at-set-once
	t.Run("started-at-set-once", func(t *testing.T) {
		s := provideCompleteSession()
		s.LastActive = time.Time{}
		s.AddTurn(timedTurn(RoleUser, base))
		s.AddTurn(timedTurn(RoleAssistant, base.Add(time.Minute)))
		assert.Equal(t, base, s.StartedAt)
	})

	// gap-above-threshold-counts-idle
	t.Run("gap-above-threshold-counts-idle", func(t *testing.T) {
		s := provideCompleteSession()
		s.LastActive = time.Time{}
		s.AddTurn(timedTurn(RoleUser, base))
		s.AddTurn(timedTurn(RoleAssistant, base.Add(10*time.Minute)))
		assert.Equal(t, 10*time.Minute, s.Idle)
	})

	// gap-below-threshold-not-idle
	t.Run("gap-below-threshold-not-idle", func(t *testing.T) {
		s := provideCompleteSession()
		s.LastActive = time.Time{}
		s.AddTurn(timedTurn(RoleUser, base))
		s.AddTurn(timedTurn(RoleAssistant, base.Add(4*time.Minute)))
		assert.Equal(t, time.Duration(0), s.Idle)
	})

	// zero-timestamp-ignored
	t.Run("zero-timestamp-ignored", func(t *testing.T) {
		s := provideCompleteSession()
		s.LastActive = time.Time{}
		s.AddTurn(timedTurn(RoleUser, base))
		s.AddTurn(&Turn{Meta: &Meta{SessionId: "id"}})
		assert.Equal(t, base, s.StartedAt)
		assert.Equal(t, base, s.LastActive)
		assert.Equal(t, time.Duration(0), s.Idle)
	})
}
