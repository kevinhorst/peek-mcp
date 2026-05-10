package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

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
		transport, _ := flags.GetString("transport")
		port, _ := flags.GetInt("port")
		depth, _ := flags.GetInt("depth")
		claudeHome, _ := flags.GetString("claude-home")
		codexHome, _ := flags.GetString("codex-home")
		diffTarget, _ := flags.GetString("diff-target")

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		store := session.NewStore(depth)
		go func() {
			watchedDir := filepath.Join(claudeHome, claude.ProjectsDir)
			err := watcher.New(session.SourceClaude, watchedDir, claude.NewParser(), store).Run(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Fatal(err)
			}
		}()

		go func() {
			watchedDir := filepath.Join(codexHome, codex.SessionDir)
			err := watcher.New(session.SourceCodex, watchedDir, codex.NewParser(), store).Run(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Fatal(err)
			}
		}()

		go func() {
			plansDir := filepath.Join(claudeHome, "plans")
			err := watcher.NewPlanWatcher(plansDir, store).Run(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Fatal(err)
			}
		}()

		go func() {
			err := watcher.NewDiffWatcher(store, diffTarget).Run(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Fatal(err)
			}
		}()

		srv := server.NewMCPServer("peek-mcp", Version(),
			server.WithToolCapabilities(true),
		)
		tools.Register(srv, store)

		switch transport {
		case "stdio":
			if err := server.ServeStdio(srv); err != nil && !errors.Is(err, context.Canceled) {
				log.Fatalf("stdio server error: %v", err)
			}
		case "http":
			httpSrv := server.NewStreamableHTTPServer(srv)

			addr := fmt.Sprintf("127.0.0.1:%d", port)
			log.Printf("peek-mcp listening on http://%s/mcp", addr)

			httpServer := &http.Server{
				Addr:    addr,
				Handler: requestLogger(httpSrv),
			}

			go func() {
				<-ctx.Done()
				httpServer.Shutdown(context.Background())
			}()

			if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("server error: %v", err)
			}
		default:
			log.Fatalf("unknown transport %q (want http or stdio)", transport)
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
	flags.String("diff-target", "develop", "Branch to diff against for session_diff")

	rootCmd.AddCommand(startCmd)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", rw.code)
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
