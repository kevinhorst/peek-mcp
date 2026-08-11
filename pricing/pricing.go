package pricing

import "strings"

type Rates struct {
	InputPerMTok      float64
	OutputPerMTok     float64
	CacheWritePerMTok float64
	CacheReadPerMTok  float64
}

var rateTable = map[string]Rates{
	"claude-fable-5":  {InputPerMTok: 5, OutputPerMTok: 25, CacheWritePerMTok: 6.25, CacheReadPerMTok: 0.50},
	"claude-opus-4":   {InputPerMTok: 15, OutputPerMTok: 75, CacheWritePerMTok: 18.75, CacheReadPerMTok: 1.50},
	"claude-sonnet-4": {InputPerMTok: 3, OutputPerMTok: 15, CacheWritePerMTok: 3.75, CacheReadPerMTok: 0.30},
	"claude-haiku-4":  {InputPerMTok: 1, OutputPerMTok: 5, CacheWritePerMTok: 1.25, CacheReadPerMTok: 0.10},
	"gpt-5.1":         {InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
	"gpt-5-codex":     {InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
	"gpt-5-mini":      {InputPerMTok: 0.25, OutputPerMTok: 2, CacheReadPerMTok: 0.025},
	"gpt-5-nano":      {InputPerMTok: 0.05, OutputPerMTok: 0.40, CacheReadPerMTok: 0.005},
	"gpt-5":           {InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
}

func Lookup(model string) (Rates, bool) {
	var bestRates Rates
	bestLength := -1
	for prefix, rates := range rateTable {
		if !strings.HasPrefix(model, prefix) {
			continue
		}
		if len(prefix) > bestLength {
			bestLength = len(prefix)
			bestRates = rates
		}
	}
	return bestRates, bestLength >= 0
}

func Cost(tokens int, ratePerMTok float64) float64 {
	return float64(tokens) / 1e6 * ratePerMTok
}
