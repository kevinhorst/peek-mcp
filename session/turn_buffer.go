package session

import (
	"bytes"
	"encoding/gob"

	"github.com/pkg/errors"
)

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

func (b *TurnBuffer) GobDecode(data []byte) error {
	snapshot := &turnBufferSnapshot{}
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(snapshot); err != nil {
		return errors.Wrap(err, "TurnBuffer.GobDecode: Failed to decode")
	}

	b.capacity = snapshot.Capacity
	b.items = snapshot.Items
	b.pushed = snapshot.Pushed
	b.pushedToolOnly = snapshot.PushedToolOnly
	return nil
}

func (b *TurnBuffer) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	snapshot := &turnBufferSnapshot{
		Capacity:       b.capacity,
		Items:          b.items,
		Pushed:         b.pushed,
		PushedToolOnly: b.pushedToolOnly,
	}
	if err := gob.NewEncoder(&buffer).Encode(snapshot); err != nil {
		return nil, errors.Wrap(err, "TurnBuffer.GobEncode: Failed to encode")
	}
	return buffer.Bytes(), nil
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

type turnBufferSnapshot struct {
	Capacity       int
	Items          []*Turn
	Pushed         int
	PushedToolOnly int
}
