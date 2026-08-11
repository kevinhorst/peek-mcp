package control

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/kevinhorst/peek-mcp/telemetry"
	"github.com/kevinhorst/peek-mcp/tools"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed assets
var assetsFS embed.FS

type Options struct {
	Store          *session.Store
	Broker         *events.Broker
	Telemetry      *telemetry.Store
	Detector       *telemetry.Detector
	Token          string
	Version        string
	Depth          int
	StartedAt      time.Time
	StateDir       *state.Dir
	Invocations    *tools.InvocationCounter
	Config         Config
	ConfigPath     string
	OverriddenKeys map[string]bool
	Restart        func()
}

type Server struct {
	store          *session.Store
	broker         *events.Broker
	telemetry      *telemetry.Store
	detector       *telemetry.Detector
	token          string
	version        string
	depth          int
	tmpl           *template.Template
	sseClients     atomic.Int64
	startedAt      time.Time
	stateDir       *state.Dir
	invocations    *tools.InvocationCounter
	config         Config
	configPath     string
	overriddenKeys map[string]bool
	restart        func()
}

func New(opts *Options) (*Server, error) {
	funcs := template.FuncMap{
		"baseName": filepath.Base,
		"add":      func(a, b int) int { return a + b },
		"ts": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"bytes": formatBytes,
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	return &Server{
		store:          opts.Store,
		broker:         opts.Broker,
		telemetry:      opts.Telemetry,
		detector:       opts.Detector,
		token:          opts.Token,
		version:        opts.Version,
		depth:          opts.Depth,
		tmpl:           tmpl,
		startedAt:      opts.StartedAt,
		stateDir:       opts.StateDir,
		invocations:    opts.Invocations,
		config:         opts.Config,
		configPath:     opts.ConfigPath,
		overriddenKeys: opts.OverriddenKeys,
		restart:        opts.Restart,
	}, nil
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func (s *Server) assetsHandler() http.Handler {
	fileServer := http.FileServerFS(assetsFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleSessionsPage)
	mux.Handle("GET /assets/", s.assetsHandler())
	mux.HandleFunc("GET /sessions/{id}", s.handleSessionDetailPage)
	mux.HandleFunc("GET /stats", s.handleStatsPage)
	mux.HandleFunc("GET /fragments/sessions", s.handleSessionsFragment)
	mux.HandleFunc("GET /fragments/stats", s.handleStatsFragment)
	mux.HandleFunc("GET /fragments/config", s.handleConfigFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/turns", s.handleTurnsFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/plan", s.handlePlanFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/diff", s.handleDiffFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/uncommitted-diff", s.handleUncommittedDiffFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/usage", s.handleUsageFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/usage/cost", s.handleUsageCostFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/usage/plans", s.handleUsagePlansFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/usage/skills", s.handleUsageSkillsFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/usage/denials", s.handleUsageDenialsFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/events", s.handleEventsFragment)
	mux.HandleFunc("GET /api/healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleSessionDetail)
	mux.HandleFunc("GET /api/sessions/{id}/turns", s.handleTurns)
	mux.HandleFunc("GET /api/sessions/{id}/plan", s.handlePlan)
	mux.HandleFunc("GET /api/sessions/{id}/diff", s.handleDiff)
	mux.HandleFunc("GET /api/sessions/{id}/uncommitted-diff", s.handleUncommittedDiff)
	mux.HandleFunc("GET /api/sessions/{id}/usage", s.handleUsage)
	mux.HandleFunc("GET /api/sessions/{id}/events", s.handleSessionEvents)
	mux.HandleFunc("POST /api/restart", s.handleRestart)
	mux.HandleFunc("POST /api/config/{key}", s.handleConfigSet)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("POST /otlp/v1/metrics", s.handleOtlpMetrics)
	return s.logRequests(s.checkHost(s.auth(mux)))
}
