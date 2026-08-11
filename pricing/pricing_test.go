package pricing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookup(t *testing.T) {
	// exact-match
	t.Run("exact-match", func(t *testing.T) {
		rates, ok := Lookup("claude-fable-5")
		require.True(t, ok)
		assert.Equal(t, 5.0, rates.InputPerMTok)
	})

	// longest-prefix-wins
	t.Run("longest-prefix-wins", func(t *testing.T) {
		rates, ok := Lookup("gpt-5.1-2026-01-15")
		require.True(t, ok)
		assert.Equal(t, Rates{InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125}, rates)

		nano, ok := Lookup("gpt-5-nano-2025")
		require.True(t, ok)
		assert.Equal(t, 0.05, nano.InputPerMTok)
	})

	// dated-model-suffix
	t.Run("dated-model-suffix", func(t *testing.T) {
		rates, ok := Lookup("claude-sonnet-4-5-20250929")
		require.True(t, ok)
		assert.Equal(t, 3.0, rates.InputPerMTok)
	})

	// unknown-model
	t.Run("unknown-model", func(t *testing.T) {
		_, ok := Lookup("opus")
		assert.False(t, ok)
	})
}

func TestCost(t *testing.T) {
	assert.Equal(t, 0.0, Cost(0, 15))
	assert.Equal(t, 15.0, Cost(1_000_000, 15))
}
