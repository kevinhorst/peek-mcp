package control

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/tools"
)

const (
	pageStats         = "stats"
	maxInstancesShown = 100
)

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
		for _, content := range s.stateDir.ReadInstances(maxInstancesShown) {
			var record tools.InstanceRecord
			if err := json.Unmarshal([]byte(content), &record); err != nil {
				continue
			}
			resp.Instances = append(resp.Instances, newInstanceView(record, resp.PID))
		}
	}
	if s.invocations != nil {
		resp.Invocations = s.invocations.Snapshot()
	}
	if s.detector != nil {
		exportStatus := s.detector.Status()
		resp.TelemetryExport = &exportStatus
	}
	return resp
}

func newInstanceView(record tools.InstanceRecord, selfPID int) instanceView {
	view := instanceView{InstanceRecord: record}
	view.Self = record.PID == selfPID
	view.Running = processAlive(record.PID)
	end := record.UpdatedAt
	if view.Running {
		end = time.Now()
	}
	view.RanFor = end.Sub(record.StartedAt).Truncate(time.Second).String()
	for _, stats := range record.Tools {
		view.TotalCount += stats.Count
		view.TotalBytes += stats.Bytes
	}
	return view
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.stats())
}

func (s *Server) handleStatsPage(w http.ResponseWriter, r *http.Request) {
	s.renderFragment(w, tmplStats, indexPage{Page: pageStats, Title: "Peek stats", BackLink: s.config.BackLink})
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
