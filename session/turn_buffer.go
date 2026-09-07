package session

import "errors"

// TurnBuffer behaves like a circular buffer if full
type TurnBuffer struct {
	capacity int
	items    []*Turn
	pushed   int
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
	b.pushed++
	if len(b.items) < b.capacity {
		b.items = append(b.items, turn)
		return
	}

	b.items = append(b.items[1:], turn)
}

func (b *TurnBuffer) Last(n int) []*Turn {
	if len(b.items) == 0 {
		return make([]*Turn, 0)
	}

	if n > len(b.items) {
		n = len(b.items)
	}

	return b.items[len(b.items)-n:]
}

func (b *TurnBuffer) Len() int {
	return len(b.items)
}

// Pushed is the total number of turns ever pushed, independent of the ring
// capacity — the buffer only retains the last `capacity` of them.
func (b *TurnBuffer) Pushed() int {
	return b.pushed
}
