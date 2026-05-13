package observability

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nixieboluo/sealos-stroage-manager/internal/config"
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
