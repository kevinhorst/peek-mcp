package tools

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvocationCounter(t *testing.T) {
	counter := NewInvocationCounter()

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.Inc("session_list")
		}()
	}
	wg.Wait()
	counter.Inc("session_get")

	snapshot := counter.Snapshot()
	assert.Equal(t, 50, snapshot["session_list"])
	assert.Equal(t, 1, snapshot["session_get"])

	snapshot["session_get"] = 99
	assert.Equal(t, 1, counter.Snapshot()["session_get"])
}
