package models

import "errors"

// TurnBuffer behaves like a circular buffer if full
type TurnBuffer struct {
	items []*Turn
	max   int
}

func NewTurnBuffer(capacity int) *TurnBuffer {
	return &TurnBuffer{
		items: make([]*Turn, 0, capacity),
		max:   capacity,
	}
}

func (b *TurnBuffer) Validate() error {
	if b == nil {
		return errors.New("turn buffer is nil")
	}

	if b.max <= 0 {
		return errors.New("turn buffer capacity must be positive")
	}

	return nil
}

func (b *TurnBuffer) Push(turn *Turn) {
	if len(b.items) < b.max {
		b.items = append(b.items, turn)
		return
	}

	noFirst := b.items[1:]
	b.items = append(noFirst, turn)
}

func (b *TurnBuffer) Last(n int) []*Turn {
	if n <= 0 || len(b.items) == 0 {
		return nil
	}
	if n > len(b.items) {
		n = len(b.items)
	}
	return b.items[len(b.items)-n:]
}

func (b *TurnBuffer) Len() int {
	return len(b.items)
}
