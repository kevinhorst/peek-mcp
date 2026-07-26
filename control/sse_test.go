package control

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func streamLines(t *testing.T, url string) (<-chan string, func()) {
	response, err := http.Get(url)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)

	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	return lines, func() { response.Body.Close() }
}

func awaitLine(t *testing.T, lines <-chan string, want string) {
	deadline := time.After(5 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("stream closed before %q", want)
			}
			if line == want {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q", want)
		}
	}
}

func TestEvents_StreamsPublishedEvents(t *testing.T) {
	server, broker := newTestServer(t, "")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	lines, closeStream := streamLines(t, ts.URL+"/api/events")
	defer closeStream()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				broker.Publish(events.Event{Type: events.TypeTurnAdded, SessionId: "s1", Agent: "claude", Ts: time.Now()})
			}
		}
	}()

	awaitLine(t, lines, "event: turn_added")
}

func TestEvents_AgentFilter(t *testing.T) {
	server, broker := newTestServer(t, "")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	lines, closeStream := streamLines(t, ts.URL+"/api/events?agent=codex")
	defer closeStream()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				broker.Publish(events.Event{Type: events.TypeTurnAdded, SessionId: "s1", Agent: "claude", Ts: time.Now()})
				broker.Publish(events.Event{Type: events.TypeDiffUpdated, SessionId: "s2", Agent: "codex", Ts: time.Now()})
			}
		}
	}()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			require.True(t, ok, "stream closed early")
			assert.NotEqual(t, "event: turn_added", line)
			if line == "event: diff_updated" {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for codex event")
		}
	}
}

func TestEvents_ClientCap(t *testing.T) {
	server, _ := newTestServer(t, "")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	responses := make([]*http.Response, 0, maxSSEClients)
	defer func() {
		for _, response := range responses {
			response.Body.Close()
		}
	}()

	for i := 0; i < maxSSEClients; i++ {
		response, err := http.Get(ts.URL + "/api/events")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		responses = append(responses, response)
	}

	overflow, err := http.Get(ts.URL + "/api/events")
	require.NoError(t, err)
	defer overflow.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, overflow.StatusCode)
	io.Copy(io.Discard, overflow.Body)
}
