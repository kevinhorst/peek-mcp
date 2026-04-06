package codex

import "errors"

type EventMessage struct {
	Type string     `json:"type"`
	Info *EventInfo `json:"info"`
}

func (m *EventMessage) Validate() error {
	if m == nil {
		return errors.New("codex event message is nil")
	}

	if m.Type == "" {
		return errors.New("type must not be empty")
	}

	if m.Type == EventTypeTokenCount && m.Info != nil && m.Info.TotalTokenUsage != nil {
		if err := m.Info.TotalTokenUsage.Validate(); err != nil {
			return err
		}
	}

	return nil
}

type EventInfo struct {
	TotalTokenUsage *TokenUsage `json:"total_token_usage"`
}

type TokenUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
	TotalTokens           int `json:"total_tokens"`
}

func (u *TokenUsage) Validate() error {
	if u == nil {
		return errors.New("codex token usage is nil")
	}

	if u.InputTokens < 0 {
		return errors.New("input_tokens must be non-negative")
	}

	if u.CachedInputTokens < 0 {
		return errors.New("cached_input_tokens must be non-negative")
	}

	if u.OutputTokens < 0 {
		return errors.New("output_tokens must be non-negative")
	}

	if u.ReasoningOutputTokens < 0 {
		return errors.New("reasoning_output_tokens must be non-negative")
	}

	if u.TotalTokens < 0 {
		return errors.New("total_tokens must be non-negative")
	}

	return nil
}
