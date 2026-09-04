package pricing

import "strings"

// AsOf is the date the embedded rates were last verified against the vendor pages.
const AsOf = "2026-09-04"

type Rates struct {
	InputPerMTok        float64
	OutputPerMTok       float64
	CacheWrite5mPerMTok float64
	CacheWrite1hPerMTok float64
	CacheReadPerMTok    float64
}

// rateTable holds one row per model version. Keys are matched as prefixes of
// the transcript model id with a boundary rule (see Lookup), so a key must be
// the full versioned id, never a family name.
//
// Sources (verified 2026-09-04):
//
//	https://platform.claude.com/docs/en/about-claude/pricing
//	https://developers.openai.com/api/docs/pricing (standard tier, <272K context)
var rateTable = map[string]Rates{
	// Anthropic — cache read 0.1× input, 5m write 1.25×, 1h write 2×
	"claude-fable-5-1":  {InputPerMTok: 10, OutputPerMTok: 50, CacheWrite5mPerMTok: 12.50, CacheWrite1hPerMTok: 20, CacheReadPerMTok: 0.25}, // read 0.025×
	"claude-fable-5":    {InputPerMTok: 10, OutputPerMTok: 50, CacheWrite5mPerMTok: 12.50, CacheWrite1hPerMTok: 20, CacheReadPerMTok: 1},
	"claude-opus-5":     {InputPerMTok: 5, OutputPerMTok: 25, CacheWrite5mPerMTok: 6.25, CacheWrite1hPerMTok: 10, CacheReadPerMTok: 0.50},
	"claude-opus-4-8":   {InputPerMTok: 5, OutputPerMTok: 25, CacheWrite5mPerMTok: 6.25, CacheWrite1hPerMTok: 10, CacheReadPerMTok: 0.50},
	"claude-opus-4-7":   {InputPerMTok: 5, OutputPerMTok: 25, CacheWrite5mPerMTok: 6.25, CacheWrite1hPerMTok: 10, CacheReadPerMTok: 0.50},
	"claude-opus-4-6":   {InputPerMTok: 5, OutputPerMTok: 25, CacheWrite5mPerMTok: 6.25, CacheWrite1hPerMTok: 10, CacheReadPerMTok: 0.50},
	"claude-opus-4-5":   {InputPerMTok: 5, OutputPerMTok: 25, CacheWrite5mPerMTok: 6.25, CacheWrite1hPerMTok: 10, CacheReadPerMTok: 0.50},
	"claude-opus-4-1":   {InputPerMTok: 15, OutputPerMTok: 75, CacheWrite5mPerMTok: 18.75, CacheWrite1hPerMTok: 30, CacheReadPerMTok: 1.50}, // retired
	"claude-opus-4":     {InputPerMTok: 15, OutputPerMTok: 75, CacheWrite5mPerMTok: 18.75, CacheWrite1hPerMTok: 30, CacheReadPerMTok: 1.50}, // retired
	"claude-sonnet-5":   {InputPerMTok: 2, OutputPerMTok: 10, CacheWrite5mPerMTok: 2.50, CacheWrite1hPerMTok: 4, CacheReadPerMTok: 0.20},
	"claude-sonnet-4-6": {InputPerMTok: 3, OutputPerMTok: 15, CacheWrite5mPerMTok: 3.75, CacheWrite1hPerMTok: 6, CacheReadPerMTok: 0.30},
	"claude-sonnet-4-5": {InputPerMTok: 3, OutputPerMTok: 15, CacheWrite5mPerMTok: 3.75, CacheWrite1hPerMTok: 6, CacheReadPerMTok: 0.30},
	"claude-sonnet-4":   {InputPerMTok: 3, OutputPerMTok: 15, CacheWrite5mPerMTok: 3.75, CacheWrite1hPerMTok: 6, CacheReadPerMTok: 0.30}, // retired
	"claude-haiku-4-5":  {InputPerMTok: 1, OutputPerMTok: 5, CacheWrite5mPerMTok: 1.25, CacheWrite1hPerMTok: 2, CacheReadPerMTok: 0.10},
	"claude-haiku-3-5":  {InputPerMTok: 0.80, OutputPerMTok: 4, CacheWrite5mPerMTok: 1, CacheWrite1hPerMTok: 1.60, CacheReadPerMTok: 0.08}, // retired
	// OpenAI — cached input only; no write tiers
	"gpt-5.6-sol":   {InputPerMTok: 4, OutputPerMTok: 20, CacheReadPerMTok: 0.40},
	"gpt-5.6-terra": {InputPerMTok: 2, OutputPerMTok: 12, CacheReadPerMTok: 0.20},
	"gpt-5.6-luna":  {InputPerMTok: 0.20, OutputPerMTok: 1.20, CacheReadPerMTok: 0.02},
	"gpt-5.5":       {InputPerMTok: 5, OutputPerMTok: 30, CacheReadPerMTok: 0.50},
	"gpt-5.4":       {InputPerMTok: 2.50, OutputPerMTok: 15, CacheReadPerMTok: 0.25},
	"gpt-5.4-mini":  {InputPerMTok: 0.75, OutputPerMTok: 4.50, CacheReadPerMTok: 0.075},
	"gpt-5.4-nano":  {InputPerMTok: 0.20, OutputPerMTok: 1.25, CacheReadPerMTok: 0.02},
	"gpt-5.3-codex": {InputPerMTok: 1.75, OutputPerMTok: 14, CacheReadPerMTok: 0.175},
	"gpt-5.2":       {InputPerMTok: 1.75, OutputPerMTok: 14, CacheReadPerMTok: 0.175},
	"gpt-5.1":       {InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
	"gpt-5":         {InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
	"gpt-5-mini":    {InputPerMTok: 0.25, OutputPerMTok: 2, CacheReadPerMTok: 0.025},
	"gpt-5-nano":    {InputPerMTok: 0.05, OutputPerMTok: 0.40, CacheReadPerMTok: 0.005},
}

// Lookup resolves a transcript model id to its rates. The longest table key
// that is a prefix of model wins, but only when the character after the prefix
// is a version/variant boundary ('-', '[') or the end of the id — so
// "gpt-5.5" never resolves to the "gpt-5" row, while "claude-opus-4-8[1m]" and
// "claude-sonnet-4-5-20250929" resolve to their versioned rows.
func Lookup(model string) (Rates, bool) {
	var bestRates Rates
	bestLength := -1
	for prefix, rates := range rateTable {
		if !strings.HasPrefix(model, prefix) || !boundaryAfter(model, len(prefix)) {
			continue
		}
		if len(prefix) > bestLength {
			bestLength = len(prefix)
			bestRates = rates
		}
	}
	return bestRates, bestLength >= 0
}

func boundaryAfter(model string, at int) bool {
	if at >= len(model) {
		return true
	}
	return model[at] == '-' || model[at] == '['
}

func Cost(tokens int, ratePerMTok float64) float64 {
	return float64(tokens) / 1e6 * ratePerMTok
}
