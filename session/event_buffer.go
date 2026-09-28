package session

import "errors"

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
