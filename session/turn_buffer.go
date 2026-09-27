package session

import "errors"

// TurnBuffer behaves like a circular buffer if full
type TurnBuffer struct {
	capacity       int
	items          []*Turn
	pushed         int
	pushedToolOnly int
}

func NewTurnBuffer(capacity int) *TurnBuffer {
	return &TurnBuffer{
		capacity: capacity,
		items:    make([]*Turn, 0, capacity),
	}
}

func (b *TurnBuffer) Validate() error {
	if b == nil {
		return errors.New("TurnBuffer.Validate: Called on nil")
	}

	// capacity
	if b.capacity <= 0 {
		return errors.New("TurnBuffer.Validate: Capacity must be positive")
	}

	return nil
}

func (b *TurnBuffer) Push(turn *Turn) {
	if turn.IsToolOnly() {
		b.pushedToolOnly++
	} else {
		b.pushed++
	}
	if len(b.items) < b.capacity {
		b.items = append(b.items, turn)
		return
	}

	b.items = append(b.items[1:], turn)
}

func (b *TurnBuffer) Len() int {
	return len(b.items)
}

// Pushed is the total number of conversational turns ever pushed, independent
// of the ring capacity; tool-only turns are retained but not counted here.
func (b *TurnBuffer) Pushed() int {
	return b.pushed
}

// PushedWithToolCalls is the total number of turns ever pushed, tool-only
// turns included, independent of the ring capacity.
func (b *TurnBuffer) PushedWithToolCalls() int {
	return b.pushed + b.pushedToolOnly
}
