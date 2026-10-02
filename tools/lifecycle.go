package tools

import (
	"sync"
	"time"
)

type LifecycleState string

const (
	StateCold    LifecycleState = "cold"
	StateWarm    LifecycleState = "warm"
	StateWarming LifecycleState = "warming"
)

// Lifecycle moves an instance between cold and warm: a tool call warms it, and the
// keepalive that starts when the last call in flight ends cools it again.
type Lifecycle struct {
	mu sync.Mutex

	cool       func()
	generation int
	inFlight   int
	isWarm     bool
	keepalive  time.Duration
	onState    func(state LifecycleState)
	timer      *time.Timer
	warm       func(window time.Duration)
	window     time.Duration
}

func NewLifecycle(
	cool func(),
	keepalive time.Duration,
	onState func(state LifecycleState),
	warm func(window time.Duration),
	window time.Duration,
) *Lifecycle {
	return &Lifecycle{
		cool:      cool,
		keepalive: keepalive,
		onState:   onState,
		warm:      warm,
		window:    window,
	}
}

func (l *Lifecycle) coolDown() {
	l.cool()
	l.isWarm = false
	l.onState(StateCold)
}

// expire runs on the keepalive timer's goroutine.
func (l *Lifecycle) expire(generation int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	isStale := generation != l.generation || !l.isWarm
	if isStale {
		return
	}
	l.coolDown()
}

func (l *Lifecycle) warmUp() {
	l.onState(StateWarming)
	l.warm(l.window)
	l.isWarm = true
}

// Acquire counts one call in flight and warms a cold instance. The caller waits on
// the store's ready signal and calls Release when the call ends.
func (l *Lifecycle) Acquire() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.inFlight++
	l.generation++
	if l.timer != nil {
		l.timer.Stop()
	}
	if !l.isWarm {
		l.warmUp()
	}
}

func (l *Lifecycle) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.inFlight--
	isIdle := l.inFlight == 0 && l.keepalive > 0
	if !isIdle {
		return
	}

	generation := l.generation
	l.timer = time.AfterFunc(l.keepalive, func() { l.expire(generation) })
}

// Start warms the instance at once when the keepalive is off; it then never cools.
func (l *Lifecycle) Start() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.keepalive > 0 {
		return
	}
	l.warmUp()
}

// Widen reloads the instance with a wider watch window and keeps it for the rest of the process.
func (l *Lifecycle) Widen(window time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	isWider := l.window > 0 && window > l.window
	if !isWider {
		return
	}

	l.window = window
	if l.isWarm {
		l.cool()
	}
	l.warmUp()
}
