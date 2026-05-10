package main

import (
	"context"
	"errors"
	"flag"
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
)

func main() {
	transport := flag.String("transport", "http", "Transport: http or stdio")
	port := flag.Int("port", 4242, "HTTP port (http transport only)")
	depth := flag.Int("depth", 20, "Ring buffer size per session (max turns kept)")
	claudeHome := flag.String("claude-home", defaultHome(".claude"), "Claude Code session root")
	codexHome := flag.String("codex-home", defaultHome(".codex"), "Codex session root")
	diffTarget := flag.String("diff-target", "develop", "Branch to diff against for session_diff")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	store := session.NewStore(*depth)
	go func() {
		watchedDir := filepath.Join(*claudeHome, claude.ProjectsDir)
		err := watcher.New(session.SourceClaude, watchedDir, claude.NewParser(), store).Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	}()

	go func() {
		watchedDir := filepath.Join(*codexHome, codex.SessionDir)
		err := watcher.New(session.SourceCodex, watchedDir, codex.NewParser(), store).Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	}()

	go func() {
		plansDir := filepath.Join(*claudeHome, "plans")
		err := watcher.NewPlanWatcher(plansDir, store).Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	}()

	go func() {
		err := watcher.NewDiffWatcher(store, *diffTarget).Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	}()

	srv := server.NewMCPServer("peek-mcp", "1.0.0",
		server.WithToolCapabilities(true),
	)
	tools.Register(srv, store)

	switch *transport {
	case "stdio":
		if err := server.ServeStdio(srv); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatalf("stdio server error: %v", err)
		}
	case "http":
		httpSrv := server.NewStreamableHTTPServer(srv)

		addr := fmt.Sprintf("127.0.0.1:%d", *port)
		log.Printf("peek-mcp listening on http://%s/mcp", addr)

		httpServer := &http.Server{
			Addr:    addr,
			Handler: httpSrv,
		}

		go func() {
			<-ctx.Done()
			httpServer.Shutdown(context.Background())
		}()

		if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	default:
		log.Fatalf("unknown transport %q (want http or stdio)", *transport)
	}
}

func defaultHome(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", name)
	}

	return filepath.Join(home, name)
}
