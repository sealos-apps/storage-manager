package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/viewer"
)

func main() {
	if err := run(); err != nil {
		slog.Error("viewer dev server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	listen := flag.String("listen", "127.0.0.1:4000", "HTTP listen address")
	configPath := flag.String("config", config.DefaultPath, "viewer YAML config path")
	flag.Parse()

	cfg, err := config.LoadFile(*configPath)
	if err != nil {
		return fmt.Errorf("loading dev server config: %w", err)
	}
	runtime, err := viewer.NewRuntime(*configPath)
	if err != nil {
		return fmt.Errorf("building viewer runtime: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cleanupDone := startCleanupLoop(ctx, runtime, cfg.Cache.PurgeInterval)
	server := &http.Server{
		Addr:              *listen,
		Handler:           routes(runtime.Handler),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutting down dev server", "error", err)
		}
	}()

	slog.Info("starting viewer dev server", "listen", *listen, "config_path", *configPath)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("running dev server: %w", err)
	}
	<-cleanupDone
	return nil
}

func routes(handler *viewer.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pvcs", method(http.MethodGet, handler.ListPVCs))
	mux.HandleFunc("/api/viewer-sessions", method(http.MethodPost, handler.CreateViewerSession))
	mux.HandleFunc("/api/viewer-sessions/", viewerSessionRoute(handler))
	mux.HandleFunc("/api/pod-sessions/", podSessionRoute(handler))
	mux.HandleFunc("/internal/filebrowser-hook/verify", method(http.MethodPost, handler.VerifyFileBrowserHook))
	mux.HandleFunc("/metrics", method(http.MethodGet, handler.Metrics))
	return mux
}

func viewerSessionRoute(handler *viewer.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet:
			handler.GetViewerSession(w, req)
		case req.Method == http.MethodPost && hasSuffixPath(req.URL.Path, "/token"):
			handler.IssueToken(w, req)
		case req.Method == http.MethodPost && hasSuffixPath(req.URL.Path, "/heartbeat"):
			handler.Heartbeat(w, req)
		case req.Method == http.MethodDelete:
			handler.CloseViewerSession(w, req)
		default:
			http.NotFound(w, req)
		}
	}
}

func podSessionRoute(handler *viewer.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handler.GetPodSession(w, req)
		case http.MethodDelete:
			handler.ClosePodSession(w, req)
		default:
			http.NotFound(w, req)
		}
	}
}

func method(want string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != want {
			http.NotFound(w, req)
			return
		}
		handler(w, req)
	}
}

func hasSuffixPath(path string, suffix string) bool {
	if len(path) < len(suffix) {
		return false
	}
	return path[len(path)-len(suffix):] == suffix
}

func startCleanupLoop(ctx context.Context, runtime *viewer.Runtime, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := runtime.Cleanup(ctx); err != nil {
					slog.Error("running scheduled viewer cleanup", "error", err)
				}
			}
		}
	}()
	return done
}
