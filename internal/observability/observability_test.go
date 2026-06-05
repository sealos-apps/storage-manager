package observability

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

func TestObserveHTTPRecordsMetricsAndStructuredLog(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	var out bytes.Buffer
	cfg := testConfig()
	cfg.Logs.Exporter = "stdout"
	cfg.Logs.Level = "info"
	recorder := MustNew(cfg, &out)
	recorder.ObserveHTTP(t.Context(), "GET", "/pvcs", 500, time.Millisecond)

	body := prometheusText(recorder)
	if !strings.Contains(body, `viewer_http_route_requests_total{Method="GET",Route="/pvcs",StatusClass="5xx"} 1`) {
		t.Fatalf("metrics missing HTTP request count: %s", body)
	}
	logLine := out.String()
	if !strings.Contains(logLine, `"route":"/pvcs"`) {
		t.Fatalf("log missing route: %s", logLine)
	}
	if strings.Contains(logLine, "kubeconfig") || strings.Contains(logLine, "token") {
		t.Fatalf("log leaked sensitive value: %s", logLine)
	}
}

func TestWritePrometheusExposesMetrics(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	recorder := MustNew(testConfig(), nil)
	recorder.ObserveHTTP(t.Context(), "GET", "/pvcs", http.StatusOK, time.Millisecond)
	recorder.ObserveHTTP(t.Context(), "GET", "/pvcs", http.StatusInternalServerError, 2*time.Millisecond)
	recorder.ObserveCleanupDeleted()

	body := prometheusText(recorder)
	if !strings.Contains(body, `viewer_http_route_requests_total{Method="GET",Route="/pvcs",StatusClass="2xx"} 1`) ||
		!strings.Contains(body, `viewer_http_route_requests_total{Method="GET",Route="/pvcs",StatusClass="5xx"} 1`) {
		t.Fatalf("metrics body missing HTTP count: %s", body)
	}
	if !strings.Contains(body, "viewer_cleanup_deleted_total 1") {
		t.Fatalf("metrics body missing cleanup count: %s", body)
	}
}

func TestWritePrometheusExposesDeclaredMetricsWhenEmpty(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	recorder := MustNew(testConfig(), nil)

	body := prometheusText(recorder)
	if !strings.Contains(body, "# HELP viewer_http_route_requests_total ") {
		t.Fatalf("metrics body missing HTTP help: %s", body)
	}
	if !strings.Contains(body, "# TYPE viewer_http_route_requests_total counter") {
		t.Fatalf("metrics body missing HTTP type: %s", body)
	}
	if !strings.Contains(body, "viewer_cleanup_deleted_total 0") {
		t.Fatalf("metrics body missing zero cleanup count: %s", body)
	}
}

func TestTraceOperationRecordsOperationMetrics(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	recorder := MustNew(testConfig(), nil)
	_, finish := recorder.TraceOperation(t.Context(), "viewer.list_pvcs")
	finish(nil)

	body := prometheusText(recorder)
	if !strings.Contains(body, `viewer_operation_requests_total{Operation="viewer.list_pvcs",Result="success"} 1`) {
		t.Fatalf("metrics body missing operation count: %s", body)
	}
	if !strings.Contains(body, `viewer_operation_duration_milliseconds_total{Operation="viewer.list_pvcs",Result="success"} `) {
		t.Fatalf("metrics body missing operation duration: %s", body)
	}
}

func TestTraceOperationExportsOTelSpan(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	recorder, err := New(
		t.Context(),
		testConfig(),
		nil,
		WithTraceSink(NewOTelTraceSink(provider)),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, finish := recorder.TraceOperation(
		t.Context(),
		"viewer.list_pvcs",
		slog.String("namespace", "default"),
		slog.Int("pvc_count", 3),
		slog.Duration("wait", 2*time.Second),
	)
	finish(nil)
	if err := provider.ForceFlush(ctx); err != nil {
		t.Fatalf("ForceFlush() error = %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name != "viewer.list_pvcs" {
		t.Fatalf("span name = %q", span.Name)
	}
	if span.SpanKind != trace.SpanKindInternal {
		t.Fatalf("span kind = %s", span.SpanKind)
	}
	if got := spanAttribute(span.Attributes, "operation.name"); got != "viewer.list_pvcs" {
		t.Fatalf("operation.name attr = %#v", got)
	}
	if got := spanAttribute(span.Attributes, "namespace"); got != "default" {
		t.Fatalf("namespace attr = %#v", got)
	}
	if got := spanAttribute(span.Attributes, "pvc_count"); got != int64(3) {
		t.Fatalf("pvc_count attr = %#v", got)
	}
	if got := spanAttribute(span.Attributes, "wait.milliseconds"); got != int64(2000) {
		t.Fatalf("wait.milliseconds attr = %#v", got)
	}
	if span.Status.Code != codes.Unset {
		t.Fatalf("span status = %s", span.Status.Code)
	}
}

func TestTraceOperationExportsOTelErrorStatus(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	recorder, err := New(
		t.Context(),
		testConfig(),
		nil,
		WithTraceSink(NewOTelTraceSink(provider)),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, finish := recorder.TraceOperation(t.Context(), "kubernetes.get")
	finish(errors.New("boom"))

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Status.Code != codes.Error {
		t.Fatalf("span status = %s", span.Status.Code)
	}
	if span.Status.Description != "boom" {
		t.Fatalf("span status description = %q", span.Status.Description)
	}
	if len(span.Events) == 0 {
		t.Fatal("span missing error event")
	}
}

func TestExtractTraceContextUsesW3CHeaders(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	headers := http.Header{}
	headers.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	headers.Set("baggage", "tenant=sealos")

	ctx := ExtractTraceContext(t.Context(), headers)
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace id = %s", spanContext.TraceID())
	}
	if spanContext.SpanID().String() != "00f067aa0ba902b7" {
		t.Fatalf("span id = %s", spanContext.SpanID())
	}
	if !spanContext.IsRemote() {
		t.Fatal("span context is not remote")
	}
	if !spanContext.IsSampled() {
		t.Fatal("span context is not sampled")
	}
	if got := baggage.FromContext(ctx).Member("tenant").Value(); got != "sealos" {
		t.Fatalf("baggage tenant = %q", got)
	}
}

func TestNewOTelTraceSinkSetsServiceResource(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(testResource("viewer-test")),
	)
	recorder, err := New(
		t.Context(),
		testConfig(),
		nil,
		WithTraceSink(NewOTelTraceSink(provider)),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, finish := recorder.TraceOperation(t.Context(), "viewer.resource")
	finish(nil)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	value, ok := spans[0].Resource.Set().Value(semconv.ServiceNameKey)
	if !ok {
		t.Fatal("span resource missing service.name")
	}
	if value.AsString() != "viewer-test" {
		t.Fatalf("service.name = %q", value.AsString())
	}
}

func testConfig() config.ObservabilityConfig {
	cfg := config.Default().Observability
	cfg.Logs.Exporter = "discard"
	cfg.Logs.Level = "error"
	return cfg
}

func prometheusText(recorder *Recorder) string {
	response := httptest.NewRecorder()
	recorder.WritePrometheus(response, httptest.NewRequest("GET", "/metrics", nil))
	return response.Body.String()
}

func TestMain(m *testing.M) {
	if err := os.Setenv("ENCORERUNTIME_NOPANIC", "1"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func spanAttribute(attrs []attribute.KeyValue, key string) any {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value.AsInterface()
		}
	}
	return nil
}

func testResource(serviceName string) *resource.Resource {
	return resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(serviceName))
}
