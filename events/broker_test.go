package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSubscribeFanout(t *testing.T) {
	b := NewBroker()
	ch1, cancel1 := b.Subscribe()
	defer cancel1()
	ch2, cancel2 := b.Subscribe()
	defer cancel2()

	b.Publish(Event{Type: TypeTurnAdded, SessionId: "s1", Agent: "claude", Ts: time.Now()})

	assert.Len(t, ch1, 1)
	assert.Len(t, ch2, 1)

	ev := <-ch1
	assert.Equal(t, TypeTurnAdded, ev.Type)
	assert.Equal(t, "s1", ev.SessionId)
	assert.Equal(t, "claude", ev.Agent)
}

func TestPublish_NoSubscribers(t *testing.T) {
	b := NewBroker()
	b.Publish(Event{Type: TypeTurnAdded, SessionId: "s1"})
}

func TestDropOnFull(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe()
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < subscriberBuffer+1; i++ {
			b.Publish(Event{Type: TypeTurnAdded, SessionId: "s1"})
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a full subscriber")
	}

	assert.Len(t, ch, subscriberBuffer)
}

func TestUnsubscribe(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe()

	cancel()
	b.Publish(Event{Type: TypeTurnAdded, SessionId: "s1"})
	assert.Len(t, ch, 0)

	cancel()
}
