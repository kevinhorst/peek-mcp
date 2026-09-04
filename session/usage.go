package session

import "errors"

type Usage struct {
	CacheCreation1hInputTokens int `json:"cache_creation_1h_input_tokens,omitempty"`
	CacheCreation5mInputTokens int `json:"cache_creation_5m_input_tokens,omitempty"`
	CacheCreationInputTokens   int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens       int `json:"cache_read_input_tokens,omitempty"`
	CachedInputTokens          int `json:"cached_input_tokens,omitempty"`
	InputTokens                int `json:"input_tokens"`
	OutputTokens               int `json:"output_tokens"`
	ReasoningOutputTokens      int `json:"reasoning_output_tokens,omitempty"`
	TotalTokens                int `json:"total_tokens,omitempty"`
}

func (u *Usage) Validate() error {
	if u == nil {
		return errors.New("usage is nil")
	}
	if u.InputTokens < 0 {
		return errors.New("input_tokens must be non-negative")
	}
	if u.OutputTokens < 0 {
		return errors.New("output_tokens must be non-negative")
	}
	if u.CachedInputTokens < 0 {
		return errors.New("cached_input_tokens must be non-negative")
	}
	if u.ReasoningOutputTokens < 0 {
		return errors.New("reasoning_output_tokens must be non-negative")
	}
	if u.TotalTokens < 0 {
		return errors.New("total_tokens must be non-negative")
	}
	if u.CacheCreation5mInputTokens < 0 {
		return errors.New("cache_creation_5m_input_tokens must be non-negative")
	}
	if u.CacheCreation1hInputTokens < 0 {
		return errors.New("cache_creation_1h_input_tokens must be non-negative")
	}
	return nil
}

func (u *Usage) Add(other *Usage) {
	if other == nil {
		return
	}
	u.InputTokens += other.InputTokens
	u.CachedInputTokens += other.CachedInputTokens
	u.OutputTokens += other.OutputTokens
	u.ReasoningOutputTokens += other.ReasoningOutputTokens
	u.TotalTokens += other.TotalTokens
	u.CacheCreationInputTokens += other.CacheCreationInputTokens
	u.CacheCreation5mInputTokens += other.CacheCreation5mInputTokens
	u.CacheCreation1hInputTokens += other.CacheCreation1hInputTokens
	u.CacheReadInputTokens += other.CacheReadInputTokens
}
