// Command openbotd is the openbot server: API, agent runtime, and web app.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/etchebarne/openbot/internal/api"
	"github.com/etchebarne/openbot/internal/attachments"
	"github.com/etchebarne/openbot/internal/bus"
	"github.com/etchebarne/openbot/internal/config"
	"github.com/etchebarne/openbot/internal/connectors"
	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/push"
	"github.com/etchebarne/openbot/internal/runtime"
	"github.com/etchebarne/openbot/internal/sandbox"
	"github.com/etchebarne/openbot/internal/secrets"
	"github.com/etchebarne/openbot/internal/settings"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/websearch"
	"github.com/etchebarne/openbot/internal/webui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println("openbotd", version)
		return
	}
	if err := run(); err != nil {
		slog.Error("openbotd exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	st, err := store.Open(ctx, cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	box, err := secrets.Open(cfg.DataDir)
	if err != nil {
		return err
	}

	set := settings.New(st, box)
	b := bus.New()
	llm := model.New(cfg.OpenCodeBaseURL, "openbot/"+strings.TrimPrefix(version, "v"), set.APIKey)
	rt := runtime.New(st, b, llm)
	rt.CompactAtTokens = cfg.CompactAtTokens
	rt.MaxSteps = cfg.MaxSteps
	windows := model.NewWindows(model.ModelsDevURL)
	go windows.Refresh()
	rt.ContextWindow = windows.Get
	rt.SecretBox = box
	rt.Timezone = set.Location
	if cfg.Sandboxes == "docker" {
		sbx := sandbox.New(sandbox.Options{Image: cfg.SandboxImage, SharedDir: filepath.Join(cfg.DataDir, "shared")})
		if sbx.Available() {
			rt.Sandboxes = sbx
		} else {
			slog.Warn("Docker isn't available; agents won't have sandboxes")
		}
	}
	notifier, err := push.New(ctx, st, box)
	if err != nil {
		return fmt.Errorf("push notifications: %w", err)
	}
	rt.Push = func(ctx context.Context, title, body, chatID string) {
		notifier.Send(ctx, push.Notification{Title: title, Body: push.Preview(body), ChatID: chatID, Tag: chatID})
	}
	if cfg.Sandboxes == "docker" {
		// Local MCP servers run in a container of their own (always the default image, which has
		// Node.js, Python and uv), apart from agents' sandboxes and the secrets agents could read.
		mcpBox := sandbox.New(sandbox.Options{SharedDir: filepath.Join(cfg.DataDir, "shared")})
		if mcpBox.Available() {
			go func() {
				if err := mcpBox.Prepare(ctx); err != nil {
					slog.Warn("preparing the sandbox image", "err", err)
				}
			}()
			connectors.SetLocalRunner(func(ctx context.Context, command string, env map[string]string) (connectors.LocalProcess, error) {
				return mcpBox.Start(ctx, "mcp", command, env)
			})
		}
	}
	switch {
	case cfg.Search == "off":
	case cfg.SearXNGURL != "":
		rt.Search = websearch.NewExternal(cfg.SearXNGURL)
	default:
		if search := websearch.NewManaged(); search.Available() {
			rt.Search = search
			go func() {
				if err := search.Prepare(ctx); err != nil {
					slog.Warn("preparing web search", "err", err)
				}
			}()
		} else {
			slog.Warn("Docker isn't available; agents won't have web search (set OPENBOT_SEARXNG_URL to use an instance of your own)")
		}
	}
	conns := connectors.NewManager(st, box)
	conns.OnSignal = rt.DeliverSignal
	rt.Connectors = conns
	// Attachments live in the shared folder, which every sandbox mounts at /shared.
	files := &attachments.Files{Dir: filepath.Join(cfg.DataDir, "shared", "attachments"), Store: st}
	rt.Files = files
	go func() {
		for {
			files.Clean(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Hour):
			}
		}
	}()
	srv := api.New(st, b, rt, llm, set, notifier, api.Options{
		PublicURL:      cfg.PublicURL,
		SecureCookies:  cfg.SecureCookies,
		AllowedOrigins: cfg.AllowedOrigins,
		Web:            webFS(cfg.WebDir),
	})

	srv.Connectors = conns
	srv.Files = files
	if err := rt.Start(ctx); err != nil {
		return err
	}
	if err := conns.Start(ctx); err != nil {
		return err
	}
	go cleanupSessions(ctx, st)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("openbotd listening", "version", version, "addr", cfg.Addr)
		errc <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	rt.Wait()
	return nil
}

// webFS picks the web app to serve: OPENBOT_WEB_DIR if set, else the copy embedded at build time.
func webFS(dir string) fs.FS {
	if dir != "" {
		return os.DirFS(dir)
	}
	if files, ok := webui.FS(); ok {
		return files
	}
	slog.Warn("no web app embedded in this build; serving the API only (use the Vite dev server)")
	return nil
}

func cleanupSessions(ctx context.Context, st *store.Store) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := st.DeleteExpiredSessions(ctx); err != nil {
				slog.Warn("cleanup sessions", "err", err)
			}
		}
	}
}
