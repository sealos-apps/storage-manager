package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
)

const (
	exporterDiscard = "discard"
	exporterEncore  = "encore"
	exporterNone    = "none"
	exporterStdout  = "stdout"
)

type HTTPLabels struct {
	Method      string
	Route       string
	StatusClass string
}

type OperationLabels struct {
	Operation string
	Result    string
}

type OperationErrorLabels struct {
	Operation string
}

type KubernetesLabels struct {
	Operation string
	Resource  string
	Result    string
}

type KubernetesErrorLabels struct {
	Operation string
	Resource  string
}

type EventLabels struct {
	Event string
}

type FileBrowserLoginLabels struct {
	Result string
}

type Metrics struct {
	httpRequests        mirroredCounterGroup[HTTPLabels]
	operationRequests   mirroredCounterGroup[OperationLabels]
	operationDurationMS mirroredCounterGroup[OperationLabels]
	operationErrors     mirroredCounterGroup[OperationErrorLabels]
	kubernetesRequests  mirroredCounterGroup[KubernetesLabels]
	kubernetesDuration  mirroredCounterGroup[KubernetesLabels]
	kubernetesErrors    mirroredCounterGroup[KubernetesErrorLabels]
	viewerSessionEvents mirroredCounterGroup[EventLabels]
	podSessionEvents    mirroredCounterGroup[EventLabels]
	authRequestEvents   mirroredCounterGroup[EventLabels]
	fileBrowserLogins   mirroredCounterGroup[FileBrowserLoginLabels]
	cleanupDeleted      mirroredCounter
}

type Counter interface {
	Add(delta uint64)
	Increment()
}

type CounterGroup[L comparable] interface {
	With(labels L) Counter
}

type MetricSources struct {
	HTTPRequests        CounterGroup[HTTPLabels]
	OperationRequests   CounterGroup[OperationLabels]
	OperationDurationMS CounterGroup[OperationLabels]
	OperationErrors     CounterGroup[OperationErrorLabels]
	KubernetesRequests  CounterGroup[KubernetesLabels]
	KubernetesDuration  CounterGroup[KubernetesLabels]
	KubernetesErrors    CounterGroup[KubernetesErrorLabels]
	ViewerSessionEvents CounterGroup[EventLabels]
	PodSessionEvents    CounterGroup[EventLabels]
	AuthRequestEvents   CounterGroup[EventLabels]
	FileBrowserLogins   CounterGroup[FileBrowserLoginLabels]
	CleanupDeleted      Counter
}

type localCounter struct {
	mu    sync.Mutex
	value uint64
}

func (c *localCounter) Add(delta uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += delta
}

func (c *localCounter) Increment() {
	c.Add(1)
}

func (c *localCounter) Value() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

type localCounterGroup[L comparable] struct {
	mu     sync.Mutex
	values map[L]*localCounter
}

func newLocalCounterGroup[L comparable]() *localCounterGroup[L] {
	return &localCounterGroup[L]{values: map[L]*localCounter{}}
}

func (g *localCounterGroup[L]) With(labels L) Counter {
	g.mu.Lock()
	defer g.mu.Unlock()
	if counter, ok := g.values[labels]; ok {
		return counter
	}
	counter := &localCounter{}
	g.values[labels] = counter
	return counter
}

func (g *localCounterGroup[L]) Values() map[L]uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[L]uint64, len(g.values))
	for labels, counter := range g.values {
		out[labels] = counter.Value()
	}
	return out
}

type noopCounter struct{}

func (noopCounter) Add(uint64) {}

func (noopCounter) Increment() {}

type mirroredCounter struct {
	encore Counter
	local  Counter
}

func (c mirroredCounter) Add(delta uint64) {
	if c.encore != nil {
		c.encore.Add(delta)
	}
	c.local.Add(delta)
}

func (c mirroredCounter) Increment() {
	c.Add(1)
}

func (c mirroredCounter) Value() uint64 {
	local, ok := c.local.(*localCounter)
	if !ok {
		return 0
	}
	return local.Value()
}

type mirroredCounterGroup[L comparable] struct {
	encore CounterGroup[L]
	local  *localCounterGroup[L]
}

func (g mirroredCounterGroup[L]) With(labels L) Counter {
	var encoreCounter Counter
	if g.encore != nil {
		encoreCounter = g.encore.With(labels)
	}
	return mirroredCounter{
		encore: encoreCounter,
		local:  g.local.With(labels),
	}
}

func (g mirroredCounterGroup[L]) Values() map[L]uint64 {
	return g.local.Values()
}

func newMirroredCounter(encore Counter) mirroredCounter {
	return mirroredCounter{
		encore: encore,
		local:  &localCounter{},
	}
}

func newMirroredCounterGroup[L comparable](encore CounterGroup[L]) mirroredCounterGroup[L] {
	return mirroredCounterGroup[L]{
		encore: encore,
		local:  newLocalCounterGroup[L](),
	}
}

type Logger struct {
	level logLevel
	sink  logSink
}

type logLevel int

const (
	logLevelDebug logLevel = iota
	logLevelInfo
	logLevelWarn
	logLevelError
)

type logSink interface {
	Enabled(level logLevel) bool
	Log(level logLevel, msg string, fields ...any)
}

type Recorder struct {
	logger  *Logger
	metrics *Metrics
}

type options struct {
	metrics MetricSources
}

type Option func(*options)

func WithMetrics(metrics MetricSources) Option {
	return func(opts *options) {
		opts.metrics = metrics
	}
}

func New(
	_ context.Context,
	cfg config.ObservabilityConfig,
	out io.Writer,
	opts ...Option,
) (*Recorder, error) {
	options := options{}
	for _, opt := range opts {
		opt(&options)
	}
	logger, err := newLogger(cfg, out)
	if err != nil {
		return nil, err
	}
	return &Recorder{
		logger:  logger,
		metrics: newMetrics(options.metrics),
	}, nil
}

func MustNew(cfg config.ObservabilityConfig, out io.Writer) *Recorder {
	recorder, err := New(context.Background(), cfg, out)
	if err != nil {
		panic(err)
	}
	return recorder
}

func (r *Recorder) Shutdown(context.Context) error {
	return nil
}

func (r *Recorder) Logger() *Logger {
	return r.logger
}

// TraceOperation records operation boundary logs into Encore's active trace.
func (r *Recorder) TraceOperation(ctx context.Context, name string, attrs ...slog.Attr) (context.Context, func(error)) {
	start := time.Now()
	fields := append([]slog.Attr{slog.String("operation", name)}, attrs...)
	r.Logger().LogAttrs(ctx, slog.LevelDebug, "operation.start", fields...)
	return ctx, func(err error) {
		duration := time.Since(start)
		result := "success"
		if err != nil {
			result = "error"
			r.metrics.operationErrors.With(OperationErrorLabels{Operation: name}).Increment()
		}
		r.metrics.operationRequests.With(OperationLabels{Operation: name, Result: result}).Increment()
		r.metrics.operationDurationMS.With(OperationLabels{Operation: name, Result: result}).
			Add(durationMilliseconds(duration))
		endFields := append(fields,
			slog.String("result", result),
			slog.Duration("duration", duration),
		)
		if err != nil {
			endFields = append(endFields, slog.String("error", err.Error()))
			r.Logger().LogAttrs(ctx, slog.LevelWarn, "operation.end", endFields...)
			return
		}
		r.Logger().LogAttrs(ctx, slog.LevelInfo, "operation.end", endFields...)
	}
}

func (r *Recorder) ObserveHTTP(ctx context.Context, method string, route string, status int, duration time.Duration) {
	r.metrics.httpRequests.With(HTTPLabels{
		Method:      method,
		Route:       route,
		StatusClass: statusClass(status),
	}).Increment()
	r.Logger().InfoContext(ctx, "http.request",
		slog.String("method", method),
		slog.String("route", route),
		slog.Int("status", status),
		slog.Duration("duration", duration),
	)
}

func (r *Recorder) ObserveKubernetes(
	operation string,
	resource string,
	err error,
	duration time.Duration,
) {
	result := "success"
	if err != nil {
		result = "error"
		r.metrics.kubernetesErrors.With(KubernetesErrorLabels{
			Operation: operation,
			Resource:  resource,
		}).Increment()
	}
	labels := KubernetesLabels{
		Operation: operation,
		Resource:  resource,
		Result:    result,
	}
	r.metrics.kubernetesRequests.With(labels).Increment()
	r.metrics.kubernetesDuration.With(labels).Add(durationMilliseconds(duration))
}

func (r *Recorder) ObserveViewerSession(event string) {
	r.metrics.viewerSessionEvents.With(EventLabels{Event: event}).Increment()
}

func (r *Recorder) ObservePodSession(event string) {
	r.metrics.podSessionEvents.With(EventLabels{Event: event}).Increment()
}

func (r *Recorder) ObserveAuthRequest(event string) {
	r.metrics.authRequestEvents.With(EventLabels{Event: event}).Increment()
}

func (r *Recorder) ObserveFileBrowserLogin(result string) {
	r.metrics.fileBrowserLogins.With(FileBrowserLoginLabels{Result: result}).Increment()
}

func (r *Recorder) ObserveCleanupDeleted() {
	r.metrics.cleanupDeleted.Increment()
}

func (r *Recorder) WritePrometheus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	r.writeLocalPrometheus(w)
}

func newLogger(cfg config.ObservabilityConfig, out io.Writer) (*Logger, error) {
	level := logLevelInfo
	switch normalized(cfg.Logs.Level) {
	case "", "info":
		level = logLevelInfo
	case "debug":
		level = logLevelDebug
	case "warn":
		level = logLevelWarn
	case "error":
		level = logLevelError
	default:
		return nil, fmt.Errorf("unsupported observability.logs.level %q", cfg.Logs.Level)
	}
	if out == nil {
		out = io.Discard
	}
	switch normalized(cfg.Logs.Exporter) {
	case "", exporterEncore:
		return &Logger{level: level, sink: newEncoreLogSink()}, nil
	case exporterStdout:
		return &Logger{level: level, sink: newSlogSink(out)}, nil
	case exporterDiscard, exporterNone:
		return &Logger{level: level, sink: newSlogSink(io.Discard)}, nil
	default:
		return nil, fmt.Errorf("unsupported observability.logs.exporter %q", cfg.Logs.Exporter)
	}
}

func (l *Logger) LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	l.log(ctx, fromSlogLevel(level), msg, attrs...)
}

func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.log(ctx, logLevelInfo, msg, argsToAttrs(args)...)
}

func (l *Logger) log(_ context.Context, level logLevel, msg string, attrs ...slog.Attr) {
	if l == nil || l.sink == nil || level < l.level || !l.sink.Enabled(level) {
		return
	}
	l.sink.Log(level, msg, attrsToFields(attrs)...)
}

type slogSink struct {
	logger *slog.Logger
}

func newSlogSink(out io.Writer) slogSink {
	return slogSink{
		logger: slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
}

func (s slogSink) Enabled(_ logLevel) bool {
	return true
}

func (s slogSink) Log(level logLevel, msg string, fields ...any) {
	switch level {
	case logLevelDebug:
		s.logger.Debug(msg, fields...)
	case logLevelInfo:
		s.logger.Info(msg, fields...)
	case logLevelWarn:
		s.logger.Warn(msg, fields...)
	case logLevelError:
		s.logger.Error(msg, fields...)
	}
}

func newMetrics(sources MetricSources) *Metrics {
	return &Metrics{
		httpRequests:        newMirroredCounterGroup[HTTPLabels](sources.HTTPRequests),
		operationRequests:   newMirroredCounterGroup[OperationLabels](sources.OperationRequests),
		operationDurationMS: newMirroredCounterGroup[OperationLabels](sources.OperationDurationMS),
		operationErrors:     newMirroredCounterGroup[OperationErrorLabels](sources.OperationErrors),
		kubernetesRequests:  newMirroredCounterGroup[KubernetesLabels](sources.KubernetesRequests),
		kubernetesDuration:  newMirroredCounterGroup[KubernetesLabels](sources.KubernetesDuration),
		kubernetesErrors:    newMirroredCounterGroup[KubernetesErrorLabels](sources.KubernetesErrors),
		viewerSessionEvents: newMirroredCounterGroup[EventLabels](sources.ViewerSessionEvents),
		podSessionEvents:    newMirroredCounterGroup[EventLabels](sources.PodSessionEvents),
		authRequestEvents:   newMirroredCounterGroup[EventLabels](sources.AuthRequestEvents),
		fileBrowserLogins:   newMirroredCounterGroup[FileBrowserLoginLabels](sources.FileBrowserLogins),
		cleanupDeleted:      newMirroredCounter(sources.CleanupDeleted),
	}
}

func (r *Recorder) writeLocalPrometheus(w io.Writer) {
	_, _ = io.WriteString(w, "# Metrics are mirrored locally for /metrics and exported by the Encore runtime according to infra-config.json.\n")
	writeLocalGroup(w, "viewer_http_route_requests_total", r.metrics.httpRequests.Values(), func(labels HTTPLabels) string {
		return prometheusLabels("Method", labels.Method, "Route", labels.Route, "StatusClass", labels.StatusClass)
	})
	writeLocalGroup(w, "viewer_operation_requests_total", r.metrics.operationRequests.Values(), func(labels OperationLabels) string {
		return prometheusLabels("Operation", labels.Operation, "Result", labels.Result)
	})
	writeLocalGroup(w, "viewer_operation_duration_milliseconds_total", r.metrics.operationDurationMS.Values(), func(labels OperationLabels) string {
		return prometheusLabels("Operation", labels.Operation, "Result", labels.Result)
	})
	writeLocalGroup(w, "viewer_operation_errors_total", r.metrics.operationErrors.Values(), func(labels OperationErrorLabels) string {
		return prometheusLabels("Operation", labels.Operation)
	})
	writeLocalGroup(w, "viewer_kubernetes_operation_requests_total", r.metrics.kubernetesRequests.Values(), func(labels KubernetesLabels) string {
		return prometheusLabels("Operation", labels.Operation, "Resource", labels.Resource, "Result", labels.Result)
	})
	writeLocalGroup(w, "viewer_kubernetes_operation_duration_milliseconds_total", r.metrics.kubernetesDuration.Values(), func(labels KubernetesLabels) string {
		return prometheusLabels("Operation", labels.Operation, "Resource", labels.Resource, "Result", labels.Result)
	})
	writeLocalGroup(w, "viewer_kubernetes_operation_errors_total", r.metrics.kubernetesErrors.Values(), func(labels KubernetesErrorLabels) string {
		return prometheusLabels("Operation", labels.Operation, "Resource", labels.Resource)
	})
	writeLocalGroup(w, "viewer_session_events_total", r.metrics.viewerSessionEvents.Values(), func(labels EventLabels) string {
		return prometheusLabels("Event", labels.Event)
	})
	writeLocalGroup(w, "viewer_pod_session_events_total", r.metrics.podSessionEvents.Values(), func(labels EventLabels) string {
		return prometheusLabels("Event", labels.Event)
	})
	writeLocalGroup(w, "viewer_auth_request_events_total", r.metrics.authRequestEvents.Values(), func(labels EventLabels) string {
		return prometheusLabels("Event", labels.Event)
	})
	writeLocalGroup(w, "viewer_filebrowser_logins_total", r.metrics.fileBrowserLogins.Values(), func(labels FileBrowserLoginLabels) string {
		return prometheusLabels("Result", labels.Result)
	})
	if value := r.metrics.cleanupDeleted.Value(); value > 0 {
		_, _ = fmt.Fprintf(w, "viewer_cleanup_deleted_total %d\n", value)
	}
}

func writeLocalGroup[L comparable](
	w io.Writer,
	name string,
	values map[L]uint64,
	labels func(L) string,
) {
	lines := make([]string, 0, len(values))
	for labelSet, value := range values {
		if value == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s%s %d\n", name, labels(labelSet), value))
	}
	sort.Strings(lines)
	for _, line := range lines {
		_, _ = io.WriteString(w, line)
	}
}

func prometheusLabels(pairs ...string) string {
	if len(pairs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, pairs[i], prometheusEscape(pairs[i+1])))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func prometheusEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

func runningUnderEncore() bool {
	return normalized(getenv("ENCORERUNTIME_NOPANIC")) == ""
}

var getenv = func(key string) string {
	return os.Getenv(key)
}

func argsToAttrs(args []any) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(args))
	for _, arg := range args {
		if attr, ok := arg.(slog.Attr); ok {
			attrs = append(attrs, attr)
		}
	}
	return attrs
}

func attrsToFields(attrs []slog.Attr) []any {
	fields := make([]any, 0, len(attrs)*2)
	for _, attr := range attrs {
		attr.Value = attr.Value.Resolve()
		fields = append(fields, attr.Key, attrValue(attr))
	}
	return fields
}

func attrValue(attr slog.Attr) any {
	switch attr.Value.Kind() {
	case slog.KindString:
		return attr.Value.String()
	case slog.KindBool:
		return attr.Value.Bool()
	case slog.KindInt64:
		return attr.Value.Int64()
	case slog.KindUint64:
		return attr.Value.Uint64()
	case slog.KindFloat64:
		return attr.Value.Float64()
	case slog.KindDuration:
		return attr.Value.Duration()
	case slog.KindTime:
		return attr.Value.Time()
	default:
		return attr.Value.Any()
	}
}

func fromSlogLevel(level slog.Level) logLevel {
	switch {
	case level < slog.LevelInfo:
		return logLevelDebug
	case level < slog.LevelWarn:
		return logLevelInfo
	case level < slog.LevelError:
		return logLevelWarn
	default:
		return logLevelError
	}
}

func statusClass(status int) string {
	if status < 100 {
		return "unknown"
	}
	return fmt.Sprintf("%dxx", status/100)
}

func durationMilliseconds(duration time.Duration) uint64 {
	ms := duration.Milliseconds()
	if ms > 0 {
		return uint64(ms)
	}
	return 1
}

func normalized(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
