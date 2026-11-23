package session

import (
	"bytes"
	"encoding/gob"

	"github.com/pkg/errors"
)

// TurnBuffer keeps the last capacity turns twice over: every turn, and the conversational ones alone.
type TurnBuffer struct {
	capacity       int
	conversational []*Turn
	items          []*Turn
	pushed         int
	pushedToolOnly int
}

func NewTurnBuffer(capacity int) *TurnBuffer {
	return &TurnBuffer{
		capacity:       capacity,
		conversational: make([]*Turn, 0, capacity),
		items:          make([]*Turn, 0, capacity),
	}
}

func (b *TurnBuffer) GobDecode(data []byte) error {
	snapshot := &turnBufferSnapshot{}
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(snapshot); err != nil {
		return errors.Wrap(err, "TurnBuffer.GobDecode: Failed to decode")
	}

	b.capacity = snapshot.Capacity
	b.items = snapshot.Items
	b.conversational = snapshot.ConversationalEvicted
	for _, turn := range b.items {
		if !turn.IsToolOnly() {
			b.conversational = append(b.conversational, turn)
		}
	}
	b.pushed = snapshot.Pushed
	b.pushedToolOnly = snapshot.PushedToolOnly
	return nil
}

// GobEncode writes the conversational turns already gone from items apart, so the turns both rings share stay shared on decode.
func (b *TurnBuffer) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	shared := 0
	for _, turn := range b.items {
		if !turn.IsToolOnly() {
			shared++
		}
	}

	evicted := max(0, len(b.conversational)-shared)
	snapshot := &turnBufferSnapshot{
		Capacity:              b.capacity,
		ConversationalEvicted: b.conversational[:evicted],
		Items:                 b.items,
		Pushed:                b.pushed,
		PushedToolOnly:        b.pushedToolOnly,
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
	b.items = pushRing(b.items, turn, b.capacity)
	if turn.IsToolOnly() {
		b.pushedToolOnly++
		return
	}

	b.pushed++
	b.conversational = pushRing(b.conversational, turn, b.capacity)
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
	Capacity              int
	ConversationalEvicted []*Turn
	Items                 []*Turn
	Pushed                int
	PushedToolOnly        int
}

// pushRing shifts in place once full, so an evicted turn leaves the backing array and can be collected.
func pushRing(ring []*Turn, turn *Turn, capacity int) []*Turn {
	if len(ring) < capacity {
		return append(ring, turn)
	}

	copy(ring, ring[1:])
	ring[len(ring)-1] = turn
	return ring
}
