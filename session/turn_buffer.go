package session

import "errors"

// TurnBuffer behaves like a circular buffer if full
type TurnBuffer struct {
	capacity int
	items    []*Turn
}

func NewTurnBuffer(capacity int) *TurnBuffer {
	return &TurnBuffer{
		capacity: capacity,
		items:    make([]*Turn, 0, capacity),
	}
}

func (b *TurnBuffer) Validate() error {
	if b == nil {
		return errors.New("turn buffer is nil")
	}

	if b.capacity <= 0 {
		return errors.New("turn buffer capacity must be positive")
	}

	return nil
}

func (b *TurnBuffer) Push(turn *Turn) {
	if len(b.items) < b.capacity {
		b.items = append(b.items, turn)
		return
	}

	b.items = append(b.items[1:], turn)
}

func (b *TurnBuffer) Last(n int) ([]*Turn, bool) {
	if len(b.items) == 0 {
		return nil, false
	}

	if n > len(b.items) {
		n = len(b.items)
	}

	return b.items[len(b.items)-n:], true
}

func (b *TurnBuffer) Len() int {
	return len(b.items)
}
