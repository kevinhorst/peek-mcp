package control

import (
	"embed"
	"html/template"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed assets
var assetsFS embed.FS

type Options struct {
	Store   *session.Store
	Broker  *events.Broker
	Token   string
	Version string
	Depth   int
}

type Server struct {
	store      *session.Store
	broker     *events.Broker
	token      string
	version    string
	depth      int
	tmpl       *template.Template
	sseClients atomic.Int64
}

func New(opts *Options) (*Server, error) {
	funcs := template.FuncMap{
		"baseName": filepath.Base,
		"ts": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	return &Server{
		store:   opts.Store,
		broker:  opts.Broker,
		token:   opts.Token,
		version: opts.Version,
		depth:   opts.Depth,
		tmpl:    tmpl,
	}, nil
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
	mux.HandleFunc("GET /fragments/sessions", s.handleSessionsFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/turns", s.handleTurnsFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/plan", s.handlePlanFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/diff", s.handleDiffFragment)
	mux.HandleFunc("GET /fragments/sessions/{id}/uncommitted-diff", s.handleUncommittedDiffFragment)
	mux.HandleFunc("GET /api/healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleSessionDetail)
	mux.HandleFunc("GET /api/sessions/{id}/turns", s.handleTurns)
	mux.HandleFunc("GET /api/sessions/{id}/plan", s.handlePlan)
	mux.HandleFunc("GET /api/sessions/{id}/diff", s.handleDiff)
	mux.HandleFunc("GET /api/sessions/{id}/uncommitted-diff", s.handleUncommittedDiff)
	mux.HandleFunc("GET /api/sessions/{id}/usage", s.handleUsage)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	return s.logRequests(s.checkHost(s.auth(mux)))
}
