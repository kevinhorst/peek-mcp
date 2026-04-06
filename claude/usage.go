package claude

import "errors"

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
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

	return nil
}
