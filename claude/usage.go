package claude

import "errors"

type Usage struct {
	InputTokens              int            `json:"input_tokens"`
	OutputTokens             int            `json:"output_tokens"`
	CacheCreationInputTokens int            `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int            `json:"cache_read_input_tokens"`
	CacheCreation            *CacheCreation `json:"cache_creation"` // optional; absent in older transcripts
}

// CacheCreation is the per-TTL breakdown of CacheCreationInputTokens.
type CacheCreation struct {
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
}

func (u *Usage) Validate() error {
	if u == nil {
		return errors.New("claude usage is nil")
	}

	if u.InputTokens < 0 {
		return errors.New("input_tokens must be non-negative")
	}

	if u.OutputTokens < 0 {
		return errors.New("output_tokens must be non-negative")
	}

	if u.CacheCreationInputTokens < 0 {
		return errors.New("cache_creation_input_tokens must be non-negative")
	}

	if u.CacheReadInputTokens < 0 {
		return errors.New("cache_read_input_tokens must be non-negative")
	}

	if u.CacheCreation != nil {
		if u.CacheCreation.Ephemeral5mInputTokens < 0 {
			return errors.New("cache_creation.ephemeral_5m_input_tokens must be non-negative")
		}
		if u.CacheCreation.Ephemeral1hInputTokens < 0 {
			return errors.New("cache_creation.ephemeral_1h_input_tokens must be non-negative")
		}
	}

	return nil
}
