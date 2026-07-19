package events

import (
	"sync"
	"time"
)

type Type string

const (
	TypeSessionCreated         Type = "session_created"
	TypeTurnAdded              Type = "turn_added"
	TypePlanUpdated            Type = "plan_updated"
	TypeDiffUpdated            Type = "diff_updated"
	TypeUncommittedDiffUpdated Type = "uncommitted_diff_updated"
)

type Event struct {
	Type      Type      `json:"type"`
	SessionId string    `json:"session_id"`
	Agent     string    `json:"agent"`
	Ts        time.Time `json:"ts"`
}

const subscriberBuffer = 16

type Broker struct {
	mu   sync.Mutex
	subs map[int]chan Event
	next int
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[int]chan Event)}
}

func (b *Broker) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.next
	b.next++
	ch := make(chan Event, subscriberBuffer)
	b.subs[id] = ch

	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subs, id)
	}
}

func (b *Broker) Publish(ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}
