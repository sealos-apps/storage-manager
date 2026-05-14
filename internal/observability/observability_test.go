package observability

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
)

func TestObserveHTTPRecordsMetricsAndStructuredLog(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	var out bytes.Buffer
	cfg := testConfig()
	cfg.Logs.Exporter = "stdout"
	cfg.Logs.Level = "info"
	recorder := MustNew(cfg, &out)
	recorder.ObserveHTTP(t.Context(), "GET", "/api/pvcs", 500, time.Millisecond)

	body := prometheusText(recorder)
	if !strings.Contains(body, `viewer_http_route_requests_total{Method="GET",Route="/api/pvcs",StatusClass="5xx"} 1`) {
		t.Fatalf("metrics missing HTTP request count: %s", body)
	}
	logLine := out.String()
	if !strings.Contains(logLine, `"route":"/api/pvcs"`) {
		t.Fatalf("log missing route: %s", logLine)
	}
	if strings.Contains(logLine, "kubeconfig") || strings.Contains(logLine, "token") {
		t.Fatalf("log leaked sensitive value: %s", logLine)
	}
}

func TestWritePrometheusExposesMetrics(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	recorder := MustNew(testConfig(), nil)
	recorder.ObserveHTTP(t.Context(), "GET", "/api/pvcs", http.StatusOK, time.Millisecond)
	recorder.ObserveHTTP(t.Context(), "GET", "/api/pvcs", http.StatusInternalServerError, 2*time.Millisecond)
	recorder.ObserveCleanupDeleted()

	body := prometheusText(recorder)
	if !strings.Contains(body, `viewer_http_route_requests_total{Method="GET",Route="/api/pvcs",StatusClass="2xx"} 1`) ||
		!strings.Contains(body, `viewer_http_route_requests_total{Method="GET",Route="/api/pvcs",StatusClass="5xx"} 1`) {
		t.Fatalf("metrics body missing HTTP count: %s", body)
	}
	if !strings.Contains(body, "viewer_cleanup_deleted_total 1") {
		t.Fatalf("metrics body missing cleanup count: %s", body)
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
	os.Setenv("ENCORERUNTIME_NOPANIC", "1")
	os.Exit(m.Run())
}
