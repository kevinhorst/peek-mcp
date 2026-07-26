package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	maxSSEClients     = 16
	heartbeatInterval = 15 * time.Second
	sseWriteTimeout   = 10 * time.Second
)

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if s.sseClients.Add(1) > maxSSEClients {
		s.sseClients.Add(-1)
		http.Error(w, "too many event streams", http.StatusTooManyRequests)
		return
	}
	defer s.sseClients.Add(-1)

	agent := r.URL.Query().Get("agent")
	ch, cancel := s.broker.Subscribe()
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)
	w.WriteHeader(http.StatusOK)
	rc.Flush()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if writeFrame(w, rc, ": heartbeat\n\n") != nil {
				return
			}
		case ev := <-ch:
			if agent != "" && ev.Agent != agent {
				continue
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if writeFrame(w, rc, fmt.Sprintf("event: %s\ndata: %s\n\n", ev.Type, data)) != nil {
				return
			}
		}
	}
}

func writeFrame(w http.ResponseWriter, rc *http.ResponseController, frame string) error {
	rc.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
	if _, err := fmt.Fprint(w, frame); err != nil {
		return err
	}
	return rc.Flush()
}
