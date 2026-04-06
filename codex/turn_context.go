package codex

import "errors"

type TurnContext struct {
	TurnID string `json:"turn_id"`
	Model  string `json:"model"`
	CWD    string `json:"cwd"`
}

func (c *TurnContext) Validate() error {
	if c == nil {
		return errors.New("codex turn context is nil")
	}
	if c.TurnID == "" && c.Model == "" && c.CWD == "" {
		return errors.New("turn context must include turn_id, model, or cwd")
	}
	return nil
}
