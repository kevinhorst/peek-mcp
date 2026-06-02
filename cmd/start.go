package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/codex"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/tools"
	"github.com/kevinhorst/peek-mcp/watcher"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:               "start",
	Short:             "Start the peek-mcp server",
	Long:              `Start the peek-mcp MCP server with the given configuration.`,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		logLevel, _ := flags.GetString("log-level")
		transport, _ := flags.GetString("transport")
		port, _ := flags.GetInt("port")
		depth, _ := flags.GetInt("depth")
		claudeHome, _ := flags.GetString("claude-home")
		codexHome, _ := flags.GetString("codex-home")
		diffTarget, _ := flags.GetString("diff-target")
		pollInterval, _ := flags.GetDuration("poll-interval")
		pollWindow, _ := flags.GetDuration("poll-window")

		level := slog.LevelInfo
		switch logLevel {
		case "debug":
			level = slog.LevelDebug
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		var agents []session.Agent
		if claudeHome != "" {
			agents = append(agents, session.AgentClaude)
		}
		if codexHome != "" {
			agents = append(agents, session.AgentCodex)
		}

		store := session.NewStore(depth, agents...)

		if claudeHome != "" {
			go func() {
				watchedDir := filepath.Join(claudeHome, claude.ProjectsDir)
				err := watcher.New(session.AgentClaude, watchedDir, claude.NewParser(), store).Run(ctx)
				if err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("claude watcher error", "err", err)
					os.Exit(1)
				}
			}()

			go func() {
				plansDir := filepath.Join(claudeHome, "plans")
				err := watcher.NewPlanWatcher(plansDir, store).Run(ctx)
				if err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("plan watcher error", "err", err)
					os.Exit(1)
				}
			}()
		}

		if codexHome != "" {
			go func() {
				watchedDir := filepath.Join(codexHome, codex.SessionDir)
				err := watcher.New(session.AgentCodex, watchedDir, codex.NewParser(), store).Run(ctx)
				if err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("codex watcher error", "err", err)
					os.Exit(1)
				}
			}()
		}

		go func() {
			err := watcher.NewDiffWatcher(store, diffTarget, pollInterval, pollWindow).Run(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("diff watcher error", "err", err)
				os.Exit(1)
			}
		}()

		srv := server.NewMCPServer("peek-mcp", Version(),
			server.WithToolCapabilities(true),
		)
		tools.Register(srv, store)

		switch transport {
		case "stdio":
			if err := server.ServeStdio(srv); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("stdio server error", "err", err)
				os.Exit(1)
			}
		case "http":
			httpSrv := server.NewStreamableHTTPServer(srv)

			addr := fmt.Sprintf("127.0.0.1:%d", port)
			slog.Info("peek-mcp listening", "addr", fmt.Sprintf("http://%s/mcp", addr))

			httpServer := &http.Server{
				Addr:    addr,
				Handler: requestLogger(httpSrv),
			}

			go func() {
				<-ctx.Done()
				httpServer.Shutdown(context.Background())
			}()

			if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				slog.Error("server error", "err", err)
				os.Exit(1)
			}
		default:
			slog.Error("unknown transport", "transport", transport)
			os.Exit(1)
		}
	},
}

func init() {
	flags := startCmd.Flags()
	flags.String("transport", "http", "Transport: http or stdio")
	flags.Int("port", 4242, "HTTP port (http transport only)")
	flags.Int("depth", 20, "Ring buffer size per session (max turns kept)")
	flags.String("claude-home", defaultHome(".claude"), "Claude Code session root")
	flags.String("codex-home", defaultHome(".codex"), "Codex session root")
	flags.String("diff-target", "main", "Branch to diff against for session_diff")
	flags.Duration("poll-interval", time.Second, "How often to recompute the live uncommitted diff (git diff HEAD)")
	flags.Duration("poll-window", time.Hour, "Only poll repos whose session was active within this window")
	flags.String("log-level", "info", "Log level: debug, info, warn, error")

	rootCmd.AddCommand(startCmd)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		r.Body = io.NopCloser(io.TeeReader(r.Body, &buf))
		rw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", rw.code)
		if buf.Len() > 0 {
			slog.Debug("http body", "body", buf.String())
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.code = code
	sw.ResponseWriter.WriteHeader(code)
}

func defaultHome(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", name)
	}
	return filepath.Join(home, name)
}
