package tools

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	lifecycleCallCool  = "cool"
	lifecycleCallWarm  = "warm"
	lifecycleKeepalive = 20 * time.Millisecond
	lifecycleQuiet     = 5 * lifecycleKeepalive
	lifecycleTick      = 2 * time.Millisecond
	lifecycleWait      = time.Second
	windowFourteenDays = 14 * 24 * time.Hour
	windowThreeDays    = 3 * 24 * time.Hour
)

// lifecycleRecorder fakes warm and cool and records every call and state report in
// order; the keepalive timer calls it from its own goroutine.
type lifecycleRecorder struct {
	mu sync.Mutex

	calls       []string
	warmWindows []time.Duration
}

func (r *lifecycleRecorder) cool() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, lifecycleCallCool)
}

func (r *lifecycleRecorder) count(call string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for _, recorded := range r.calls {
		if recorded == call {
			count++
		}
	}
	return count
}

func (r *lifecycleRecorder) onState(state LifecycleState) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, string(state))
}

func (r *lifecycleRecorder) recordedCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string{}, r.calls...)
}

func (r *lifecycleRecorder) recordedWindows() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]time.Duration{}, r.warmWindows...)
}

func (r *lifecycleRecorder) warm(window time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, lifecycleCallWarm)
	r.warmWindows = append(r.warmWindows, window)
}

func provideLifecycle(keepalive, window time.Duration) (*Lifecycle, *lifecycleRecorder) {
	recorder := &lifecycleRecorder{}
	lifecycle := NewLifecycle(
		recorder.cool,
		keepalive,
		recorder.onState,
		recorder.warm,
		window,
	)
	return lifecycle, recorder
}

func TestLifecycle(t *testing.T) {
	// acquire-warms-cold-instance
	t.Run("acquire-warms-cold-instance", func(t *testing.T) {
		lifecycle, recorder := provideLifecycle(lifecycleKeepalive, windowThreeDays)

		lifecycle.Start()
		assert.Empty(t, recorder.recordedCalls())

		lifecycle.Acquire()
		assert.Equal(t, []string{string(StateWarming), lifecycleCallWarm}, recorder.recordedCalls())
		assert.Equal(t, []time.Duration{windowThreeDays}, recorder.recordedWindows())
	})

	// second-acquire-does-not-warm-again
	t.Run("second-acquire-does-not-warm-again", func(t *testing.T) {
		lifecycle, recorder := provideLifecycle(lifecycleKeepalive, windowThreeDays)

		lifecycle.Acquire()
		lifecycle.Acquire()
		assert.Equal(t, 1, recorder.count(lifecycleCallWarm))
		assert.Equal(t, 0, recorder.count(lifecycleCallCool))
	})

	// release-starts-keepalive-and-cools
	t.Run("release-starts-keepalive-and-cools", func(t *testing.T) {
		lifecycle, recorder := provideLifecycle(lifecycleKeepalive, windowThreeDays)

		lifecycle.Acquire()
		lifecycle.Release()
		assert.Eventually(t, func() bool { return recorder.count(lifecycleCallCool) == 1 }, lifecycleWait, lifecycleTick)

		lifecycle.Acquire()
		assert.Equal(t, 2, recorder.count(lifecycleCallWarm))
	})

	// acquire-during-keepalive-cancels
	t.Run("acquire-during-keepalive-cancels", func(t *testing.T) {
		lifecycle, recorder := provideLifecycle(lifecycleKeepalive, windowThreeDays)

		lifecycle.Acquire()
		lifecycle.Release()
		lifecycle.Acquire()
		assert.Never(t, func() bool { return recorder.count(lifecycleCallCool) > 0 }, lifecycleQuiet, lifecycleTick)
		assert.Equal(t, 1, recorder.count(lifecycleCallWarm))
	})

	// call-in-flight-blocks-cool
	t.Run("call-in-flight-blocks-cool", func(t *testing.T) {
		lifecycle, recorder := provideLifecycle(lifecycleKeepalive, windowThreeDays)

		lifecycle.Acquire()
		lifecycle.Acquire()
		lifecycle.Release()
		assert.Never(t, func() bool { return recorder.count(lifecycleCallCool) > 0 }, lifecycleQuiet, lifecycleTick)

		lifecycle.Release()
		assert.Eventually(t, func() bool { return recorder.count(lifecycleCallCool) == 1 }, lifecycleWait, lifecycleTick)
	})

	// stale-timer-ignored
	t.Run("stale-timer-ignored", func(t *testing.T) {
		lifecycle, recorder := provideLifecycle(lifecycleKeepalive, windowThreeDays)

		lifecycle.Acquire()
		lifecycle.Release()
		lifecycle.Acquire()
		lifecycle.expire(lifecycle.generation - 1)
		assert.Equal(t, 0, recorder.count(lifecycleCallCool))

		lifecycle.Release()
		assert.Eventually(t, func() bool { return recorder.count(lifecycleCallCool) == 1 }, lifecycleWait, lifecycleTick)

		lifecycle.expire(lifecycle.generation)
		assert.Equal(t, 1, recorder.count(lifecycleCallCool))
	})

	// keepalive-zero-warms-at-start-never-cools
	t.Run("keepalive-zero-warms-at-start-never-cools", func(t *testing.T) {
		lifecycle, recorder := provideLifecycle(0, windowFourteenDays)

		lifecycle.Start()
		assert.Equal(t, []string{string(StateWarming), lifecycleCallWarm}, recorder.recordedCalls())

		lifecycle.Acquire()
		lifecycle.Release()
		assert.Never(t, func() bool { return recorder.count(lifecycleCallCool) > 0 }, lifecycleQuiet, lifecycleTick)
		assert.Equal(t, 1, recorder.count(lifecycleCallWarm))
	})

	// widen-reloads-with-wider-window
	t.Run("widen-reloads-with-wider-window", func(t *testing.T) {
		lifecycle, recorder := provideLifecycle(lifecycleKeepalive, windowThreeDays)

		lifecycle.Acquire()
		lifecycle.Widen(windowFourteenDays)
		expectedCalls := []string{
			string(StateWarming),
			lifecycleCallWarm,
			lifecycleCallCool,
			string(StateWarming),
			lifecycleCallWarm,
		}
		assert.Equal(t, expectedCalls, recorder.recordedCalls())
		assert.Equal(t, []time.Duration{windowThreeDays, windowFourteenDays}, recorder.recordedWindows())

		lifecycle.Release()
		assert.Eventually(t, func() bool { return recorder.count(lifecycleCallCool) == 2 }, lifecycleWait, lifecycleTick)

		lifecycle.Acquire()
		expectedWindows := []time.Duration{windowThreeDays, windowFourteenDays, windowFourteenDays}
		assert.Equal(t, expectedWindows, recorder.recordedWindows())
	})

	// widen-not-wider-noop
	t.Run("widen-not-wider-noop", func(t *testing.T) {
		lifecycle, recorder := provideLifecycle(lifecycleKeepalive, windowFourteenDays)

		lifecycle.Acquire()
		lifecycle.Widen(windowThreeDays)
		lifecycle.Widen(windowFourteenDays)
		assert.Equal(t, []string{string(StateWarming), lifecycleCallWarm}, recorder.recordedCalls())
		assert.Equal(t, []time.Duration{windowFourteenDays}, recorder.recordedWindows())
	})

	// widen-from-everything-noop
	t.Run("widen-from-everything-noop", func(t *testing.T) {
		lifecycle, recorder := provideLifecycle(lifecycleKeepalive, 0)

		lifecycle.Acquire()
		lifecycle.Widen(windowFourteenDays)
		assert.Equal(t, []string{string(StateWarming), lifecycleCallWarm}, recorder.recordedCalls())
		assert.Equal(t, []time.Duration{0}, recorder.recordedWindows())
	})

	// state-reports-in-order
	t.Run("state-reports-in-order", func(t *testing.T) {
		lifecycle, recorder := provideLifecycle(lifecycleKeepalive, windowThreeDays)

		lifecycle.Acquire()
		lifecycle.Release()
		assert.Eventually(t, func() bool { return recorder.count(lifecycleCallCool) == 1 }, lifecycleWait, lifecycleTick)
		lifecycle.Acquire()

		expectedCalls := []string{
			string(StateWarming),
			lifecycleCallWarm,
			lifecycleCallCool,
			string(StateCold),
			string(StateWarming),
			lifecycleCallWarm,
		}
		assert.Equal(t, expectedCalls, recorder.recordedCalls())
	})
}
