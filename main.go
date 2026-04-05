package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/kevinhorst/peek-mcp/store"
	"github.com/kevinhorst/peek-mcp/tools"
	"github.com/kevinhorst/peek-mcp/watcher"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	port := flag.Int("port", 4242, "HTTP port")
	depth := flag.Int("depth", 20, "Ring buffer depth per session (max turns kept)")
	claudeHome := flag.String("claude-home", defaultHome(".claude"), "Claude Code session root")
	codexHome := flag.String("codex-home", defaultHome(".codex"), "Codex session root")
	flag.Parse()

	s := store.New(*depth)

	w := watcher.New(s, *claudeHome, *codexHome)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	go func() {
		if err := w.Run(ctx); err != nil && err != context.Canceled {
			log.Printf("watcher stopped: %v", err)
		}
	}()

	srv := server.NewMCPServer("peek-mcp", "1.0.0",
		server.WithToolCapabilities(true),
	)
	tools.Register(srv, s)

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

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func defaultHome(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", name)
	}
	return filepath.Join(home, name)
}
