package codex

import "errors"

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (b *ContentBlock) Validate() error {
	if b == nil {
		return errors.New("codex content block is nil")
	}

	if b.Type == "" {
		return errors.New("type must not be empty")
	}

	return nil
}
