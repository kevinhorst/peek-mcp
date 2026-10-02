package session

import (
	"bytes"
	"encoding/gob"

	"github.com/pkg/errors"
)

// EventBuffer holds a session's events in arrival order within two budgets:
// permission denials and every other kind. A kind keeps its first events up
// to its budget; later ones are dropped, so no kind evicts another.
type EventBuffer struct {
	capacity     int
	denialBudget int
	denials      int
	items        []*Event
}

func NewEventBuffer(capacity, denialBudget int) *EventBuffer {
	return &EventBuffer{
		capacity:     capacity,
		denialBudget: denialBudget,
	}
}

func (b *EventBuffer) GobDecode(data []byte) error {
	snapshot := &eventBufferSnapshot{}
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(snapshot); err != nil {
		return errors.Wrap(err, "EventBuffer.GobDecode: Failed to decode")
	}

	b.capacity = snapshot.Capacity
	b.denialBudget = snapshot.DenialBudget
	b.denials = snapshot.Denials
	b.items = snapshot.Items
	return nil
}

func (b *EventBuffer) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	snapshot := &eventBufferSnapshot{
		Capacity:     b.capacity,
		DenialBudget: b.denialBudget,
		Denials:      b.denials,
		Items:        b.items,
	}
	if err := gob.NewEncoder(&buffer).Encode(snapshot); err != nil {
		return nil, errors.Wrap(err, "EventBuffer.GobEncode: Failed to encode")
	}
	return buffer.Bytes(), nil
}

func (b *EventBuffer) Validate() error {
	if b == nil {
		return errors.New("EventBuffer.Validate: Called on nil")
	}

	// capacity
	if b.capacity <= 0 {
		return errors.New("EventBuffer.Validate: Capacity must be positive")
	}

	// denialBudget
	if b.denialBudget < 0 || b.denialBudget > b.capacity {
		return errors.New("EventBuffer.Validate: Denial budget must lie within capacity")
	}

	return nil
}

func (b *EventBuffer) All() []*Event {
	all := make([]*Event, len(b.items))
	copy(all, b.items)
	return all
}

func (b *EventBuffer) Len() int {
	return len(b.items)
}

func (b *EventBuffer) Push(event *Event) {
	if event.Kind == EventKindPermissionDenied {
		if b.denials >= b.denialBudget {
			return
		}
		b.denials++
		b.items = append(b.items, event)
		return
	}

	if len(b.items)-b.denials >= b.capacity-b.denialBudget {
		return
	}
	b.items = append(b.items, event)
}

type eventBufferSnapshot struct {
	Capacity     int
	DenialBudget int
	Denials      int
	Items        []*Event
}
