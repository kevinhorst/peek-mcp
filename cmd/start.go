package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/codex"
	"github.com/kevinhorst/peek-mcp/config"
	"github.com/kevinhorst/peek-mcp/control"
	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/kevinhorst/peek-mcp/telemetry"
	"github.com/kevinhorst/peek-mcp/tools"
	"github.com/kevinhorst/peek-mcp/watcher"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// TODO: abort if neither claudeHome nor codexHome are set
var startCmd = &cobra.Command{
	Use:               "start",
	Short:             "Start the peek-mcp server",
	Long:              `Start the peek-mcp MCP server with the given configuration.`,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	Run: func(cmd *cobra.Command, args []string) {
		startedAt := time.Now()
		applyEnvFallbacks(cmd)
		overriddenKeys := changedConfigKeys(cmd)
		configPath := config.DefaultPath()
		configFile, err := config.Load(configPath)
		if err != nil {
			slog.Warn("start: Ignoring invalid config file", "path", configPath, "err", err)
			configFile = &config.File{}
		}
		applyConfigFileFallbacks(cmd, configFile)
		warnMaxOutputTokens()
		flags := cmd.Flags()
		logLevel, _ := flags.GetString("log-level")
		transport, _ := flags.GetString("transport")
		port, _ := flags.GetInt("port")
		depth, _ := flags.GetInt("depth")
		claudeHome, _ := flags.GetString("claude-home")
		codexHome, _ := flags.GetString("codex-home")
		pollInterval, _ := flags.GetDuration("poll-interval")
		pollWindow, _ := flags.GetDuration("poll-window")
		stateDirPath, _ := flags.GetString("state-dir")
		stateRetentionDays, _ := flags.GetInt("state-retention-days")
		controlPort, _ := flags.GetInt("control-port")
		controlToken, _ := flags.GetString("control-token")
		backLink, _ := flags.GetString("back-link")

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

		broker := events.NewBroker()
		store := session.NewStore(depth, broker, agents...)

		var telemetryStore *telemetry.Store
		if controlPort > 0 {
			telemetryStore = telemetry.NewStore()
		}

		var stateDir *state.Dir
		if stateDirPath != "" {
			stateDir = state.NewDir(stateDirPath)
			store.StateDir = stateDir
			go runStateGc(ctx, stateDir, stateRetentionDays)
		}
		if telemetryStore != nil {
			telemetryStore.StateDir = stateDir
		}

		if claudeHome != "" {
			go func() {
				watchedDir := filepath.Join(claudeHome, claude.ProjectsDir)
				newParser := func() watcher.Parser { return claude.NewParser() }
				err := watcher.New(session.AgentClaude, watchedDir, newParser, store).Run(ctx)
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
				newParser := func() watcher.Parser { return codex.NewParser() }
				err := watcher.New(session.AgentCodex, watchedDir, newParser, store).Run(ctx)
				if err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("codex watcher error", "err", err)
					os.Exit(1)
				}
			}()

			go func() {
				err := watcher.NewCodexIndexWatcher(codexHome, store).Run(ctx)
				if err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("codex index watcher error", "err", err)
					os.Exit(1)
				}
			}()
		}

		go func() {
			err := watcher.NewDiffWatcher(store, broker, pollInterval, pollWindow, stateDir).Run(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("diff watcher error", "err", err)
				os.Exit(1)
			}
		}()

		info := tools.InstanceInfo{
			PID:       os.Getpid(),
			PPID:      os.Getppid(),
			Transport: transport,
			Version:   Version(),
			StartedAt: startedAt,
		}
		invocations := tools.NewInvocationCounter(info, stateDir)
		invocations.Persist()
		defer invocations.Persist()

		hooks := &server.Hooks{}
		hooks.AddAfterInitialize(func(ctx context.Context, id any, message *mcp.InitializeRequest, result *mcp.InitializeResult) {
			invocations.AddClient(strings.TrimSpace(message.Params.ClientInfo.Name + " " + message.Params.ClientInfo.Version))
		})
		srv := server.NewMCPServer("peek-mcp", Version(),
			server.WithToolCapabilities(true),
			server.WithResourceCapabilities(false, true),
			server.WithHooks(hooks),
		)

		var detector *telemetry.Detector
		if controlPort > 0 {
			controlLn, err := listenLoopback(controlPort, controlPort+controlPortSpan-1)
			if err != nil {
				slog.Error("control server error", "err", err)
				os.Exit(1)
			}

			boundPort := controlLn.Addr().(*net.TCPAddr).Port
			if claudeHome != "" {
				detector = telemetry.NewDetector(boundPort, filepath.Join(claudeHome, "settings.json"))
				exportStatus := detector.Status()
				slog.Info("telemetry export", "status", exportStatus.State, "detail", exportStatus.Detail)
			}

			controlOpts := &control.Options{
				Store:          store,
				Broker:         broker,
				Telemetry:      telemetryStore,
				Detector:       detector,
				Token:          controlToken,
				Version:        Version(),
				Depth:          depth,
				StartedAt:      startedAt,
				StateDir:       stateDir,
				Invocations:    invocations,
				ConfigPath:     configPath,
				OverriddenKeys: overriddenKeys,
				Config: control.Config{
					Transport:          transport,
					Port:               port,
					Depth:              depth,
					ClaudeHome:         claudeHome,
					CodexHome:          codexHome,
					PollInterval:       pollInterval.String(),
					PollWindow:         pollWindow.String(),
					StateDir:           stateDirPath,
					StateRetentionDays: stateRetentionDays,
					ControlPort:        boundPort,
					TokenSet:           controlToken != "",
					LogLevel:           logLevel,
					BackLink:           backLink,
				},
			}
			if transport == "http" {
				controlOpts.Restart = func() {
					exe, err := os.Executable()
					if err != nil {
						slog.Error("restart: executable lookup failed", "err", err)
						return
					}
					if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
						slog.Error("restart: exec failed", "err", err)
					}
				}
			}
			controlServer, err := control.New(controlOpts)
			if err != nil {
				slog.Error("control server init error", "err", err)
				os.Exit(1)
			}

			controlHTTP := &http.Server{Handler: controlServer.Handler()}
			go func() {
				<-ctx.Done()
				controlHTTP.Shutdown(context.Background())
			}()
			go func() {
				slog.Info("control server listening", "addr", "http://"+controlLn.Addr().String())
				if err := controlHTTP.Serve(controlLn); !errors.Is(err, http.ErrServerClosed) {
					slog.Error("control server error", "err", err)
					os.Exit(1)
				}
			}()
		}

		tools.Register(srv, store, invocations, telemetryStore, detector)

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
	flags.Duration("poll-interval", time.Second*5, "How often to recompute the live uncommitted diff (git diff HEAD)")
	flags.Duration("poll-window", time.Hour, "Only poll repos whose session was active within this window")
	flags.String("state-dir", filepath.Join(defaultHome(".peek"), "state"), "State directory for diff pins/snapshots and plan revisions (empty disables persistence)")
	flags.Int("state-retention-days", 90, "Days to keep per-session state before GC removes it (0 disables GC)")
	flags.Int("control-port", controlPortBase, "Control server start port; walks up to +57 if taken (dashboard + JSON API + SSE); 0 disables")
	flags.String("control-token", "", "Optional bearer token protecting the control server")
	flags.String("back-link", "", "URL of an external dashboard the control server nav links back to (empty hides the link)")
	flags.String("log-level", "info", "Log level: debug, info, warn, error")

	rootCmd.AddCommand(startCmd)
}

func runStateGc(ctx context.Context, stateDir *state.Dir, retentionDays int) {
	if retentionDays <= 0 {
		return
	}

	retention := time.Duration(retentionDays) * 24 * time.Hour
	stateDir.Gc(retention)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stateDir.Gc(retention)
		}
	}
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

var envFallbacks = map[string]string{
	"transport":            "PEEK_TRANSPORT",
	"port":                 "PEEK_PORT",
	"depth":                "PEEK_DEPTH",
	"claude-home":          "PEEK_CLAUDE_HOME",
	"codex-home":           "PEEK_CODEX_HOME",
	"poll-interval":        "PEEK_POLL_INTERVAL",
	"poll-window":          "PEEK_POLL_WINDOW",
	"state-dir":            "PEEK_STATE_DIR",
	"state-retention-days": "PEEK_STATE_RETENTION_DAYS",
	"control-port":         "PEEK_CONTROL_PORT",
	"control-token":        "PEEK_CONTROL_TOKEN",
	"back-link":            "PEEK_BACK_LINK",
	"log-level":            "PEEK_LOG_LEVEL",
}

var pathFlags = map[string]bool{
	"claude-home": true,
	"codex-home":  true,
	"state-dir":   true,
}

const recommendedMaxOutputTokens = 125_000

func warnMaxOutputTokens() {
	val, ok := os.LookupEnv("MAX_MCP_OUTPUT_TOKENS")
	if !ok {
		slog.Warn("MAX_MCP_OUTPUT_TOKENS is not set; large responses may be truncated — run 'peek-mcp setup' to configure automatically")
		return
	}
	if n, err := strconv.Atoi(val); err == nil && n < recommendedMaxOutputTokens {
		slog.Warn("MAX_MCP_OUTPUT_TOKENS is below recommended minimum", "current", n, "recommended", recommendedMaxOutputTokens)
	}
}

func applyEnvFallbacks(cmd *cobra.Command) {
	for flag, env := range envFallbacks {
		if !cmd.Flags().Changed(flag) {
			if val, ok := os.LookupEnv(env); ok {
				cmd.Flags().Set(flag, val)
			}
		}
	}
	// Expand ~ and $HOME in path flags regardless of source (flag, env, or manifest).
	for flag := range pathFlags {
		val, _ := cmd.Flags().GetString(flag)
		if expanded := expandHome(val); expanded != val {
			cmd.Flags().Set(flag, expanded)
		}
	}
}

func applyConfigFileFallbacks(cmd *cobra.Command, file *config.File) {
	for flag, value := range file.FlagValues() {
		if !cmd.Flags().Changed(flag) {
			cmd.Flags().Set(flag, value)
		}
	}
}

func changedConfigKeys(cmd *cobra.Command) map[string]bool {
	changed := make(map[string]bool)
	for _, key := range config.EditableKeys {
		changed[key] = cmd.Flags().Changed(key)
	}
	return changed
}

func defaultHome(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", name)
	}
	return filepath.Join(home, name)
}
