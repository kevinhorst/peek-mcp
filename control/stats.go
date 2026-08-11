package control

import (
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

const pageStats = "stats"

func (s *Server) stats() statsResponse {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	counts := sessionCounts{}
	s.store.WithSessions([]session.Agent{session.AgentClaude}, func(sessions []*session.Session) {
		counts.Claude = len(sessions)
	})
	s.store.WithSessions([]session.Agent{session.AgentCodex}, func(sessions []*session.Session) {
		counts.Codex = len(sessions)
	})
	counts.Total = counts.Claude + counts.Codex

	resp := statsResponse{
		PID:              os.Getpid(),
		Version:          s.version,
		StartedAt:        s.startedAt,
		Uptime:           time.Since(s.startedAt).Truncate(time.Second).String(),
		HeapAllocBytes:   int64(mem.HeapAlloc),
		SysBytes:         int64(mem.Sys),
		Goroutines:       runtime.NumGoroutine(),
		Sessions:         counts,
		SSEClients:       s.sseClients.Load(),
		Config:           s.config,
		RestartAvailable: s.restart != nil,
	}
	if s.stateDir != nil {
		resp.StateDiskBytes = s.stateDir.Size()
	}
	if s.invocations != nil {
		resp.Invocations = s.invocations.Snapshot()
	}
	return resp
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.stats())
}

func (s *Server) handleStatsPage(w http.ResponseWriter, r *http.Request) {
	s.renderFragment(w, tmplStats, indexPage{Page: pageStats, Title: "Peek stats"})
}

func (s *Server) handleStatsFragment(w http.ResponseWriter, r *http.Request) {
	s.renderFragment(w, tmplStatsFragment, s.stats())
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if s.restart == nil {
		respondBadRequest("restart not available for this transport", w)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	go func() {
		time.Sleep(250 * time.Millisecond)
		s.restart()
	}()
}
