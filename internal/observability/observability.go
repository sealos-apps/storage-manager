package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
)

type Metrics struct {
	HTTPRequests       atomic.Int64
	HTTPErrors         atomic.Int64
	ViewerCreated      atomic.Int64
	ViewerClosed       atomic.Int64
	PodCreated         atomic.Int64
	PodReused          atomic.Int64
	PodDeleted         atomic.Int64
	AuthCreated        atomic.Int64
	AuthConsumed       atomic.Int64
	AuthDenied         atomic.Int64
	FileBrowserLogins  atomic.Int64
	FileBrowserErrors  atomic.Int64
	KubernetesRequests atomic.Int64
	KubernetesErrors   atomic.Int64
	CleanupDeleted     atomic.Int64
}

type Recorder struct {
	logger  *slog.Logger
	metrics *Metrics
}

func New(cfg config.ObservabilityConfig, out io.Writer) *Recorder {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	if out == nil {
		out = io.Discard
	}
	logger := slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level}))
	return &Recorder{
		logger:  logger,
		metrics: &Metrics{},
	}
}

func (r *Recorder) Logger() *slog.Logger {
	if r == nil || r.logger == nil {
		return slog.Default()
	}
	return r.logger
}

func (r *Recorder) Metrics() *Metrics {
	if r == nil || r.metrics == nil {
		return &Metrics{}
	}
	return r.metrics
}

func (r *Recorder) StartSpan(ctx context.Context, name string, attrs ...slog.Attr) (context.Context, func(error)) {
	start := time.Now()
	r.Logger().LogAttrs(
		ctx,
		slog.LevelDebug,
		"span.start",
		append([]slog.Attr{slog.String("span", name)}, attrs...)...,
	)
	return ctx, func(err error) {
		fields := []slog.Attr{
			slog.String("span", name),
			slog.Duration("duration", time.Since(start)),
		}
		fields = append(fields, attrs...)
		if err != nil {
			fields = append(fields, slog.String("error", err.Error()))
			r.Logger().LogAttrs(ctx, slog.LevelWarn, "span.end", fields...)
			return
		}
		r.Logger().LogAttrs(ctx, slog.LevelDebug, "span.end", fields...)
	}
}

func (r *Recorder) ObserveHTTP(ctx context.Context, method string, route string, status int, duration time.Duration) {
	r.Metrics().HTTPRequests.Add(1)
	if status >= 400 {
		r.Metrics().HTTPErrors.Add(1)
	}
	r.Logger().InfoContext(ctx, "http.request",
		slog.String("method", method),
		slog.String("route", route),
		slog.Int("status", status),
		slog.Duration("duration", duration),
	)
}

func (r *Recorder) WritePrometheus(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	metrics := r.Metrics()
	writeMetric(w, "viewer_http_requests_total", "Total HTTP requests.", metrics.HTTPRequests.Load())
	writeMetric(w, "viewer_http_errors_total", "Total HTTP requests with error status.", metrics.HTTPErrors.Load())
	writeMetric(w, "viewer_sessions_created_total", "Total viewer sessions created.", metrics.ViewerCreated.Load())
	writeMetric(w, "viewer_sessions_closed_total", "Total viewer sessions closed or expired.", metrics.ViewerClosed.Load())
	writeMetric(w, "viewer_pod_sessions_created_total", "Total pod sessions created.", metrics.PodCreated.Load())
	writeMetric(w, "viewer_pod_sessions_reused_total", "Total pod sessions reused.", metrics.PodReused.Load())
	writeMetric(w, "viewer_pod_sessions_deleted_total", "Total pod sessions deleted.", metrics.PodDeleted.Load())
	writeMetric(w, "viewer_auth_requests_created_total", "Total auth requests created.", metrics.AuthCreated.Load())
	writeMetric(w, "viewer_auth_requests_consumed_total", "Total auth requests consumed.", metrics.AuthConsumed.Load())
	writeMetric(w, "viewer_auth_requests_denied_total", "Total auth requests denied.", metrics.AuthDenied.Load())
	writeMetric(w, "viewer_filebrowser_logins_total", "Total successful File Browser logins.", metrics.FileBrowserLogins.Load())
	writeMetric(w, "viewer_filebrowser_errors_total", "Total File Browser login errors.", metrics.FileBrowserErrors.Load())
	writeMetric(w, "viewer_kubernetes_requests_total", "Total Kubernetes API requests.", metrics.KubernetesRequests.Load())
	writeMetric(w, "viewer_kubernetes_errors_total", "Total Kubernetes API request errors.", metrics.KubernetesErrors.Load())
	writeMetric(w, "viewer_cleanup_deleted_total", "Total resources deleted by cleanup.", metrics.CleanupDeleted.Load())
}

func writeMetric(w io.Writer, name string, help string, value int64) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	_, _ = fmt.Fprintf(w, "# TYPE %s counter\n", name)
	_, _ = fmt.Fprintf(w, "%s %d\n", name, value)
}
