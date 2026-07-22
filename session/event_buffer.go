package session

import "errors"

type EventBuffer struct {
	capacity int
	items    []*Event
}

func NewEventBuffer(capacity int) *EventBuffer {
	return &EventBuffer{
		capacity: capacity,
		items:    make([]*Event, 0, capacity),
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
	if len(b.items) < b.capacity {
		b.items = append(b.items, event)
		return
	}

	b.items = append(b.items[1:], event)
}
