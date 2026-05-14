package observability

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
)

func TestObserveHTTPRecordsMetricsAndStructuredLog(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	recorder := New(config.ObservabilityConfig{LogLevel: "info"}, &out)
	recorder.ObserveHTTP(t.Context(), "GET", "/api/pvcs", 500, time.Millisecond)

	if recorder.Metrics().HTTPRequests.Load() != 1 {
		t.Fatal("http request metric not incremented")
	}
	if recorder.Metrics().HTTPErrors.Load() != 1 {
		t.Fatal("http error metric not incremented")
	}
	logLine := out.String()
	if !strings.Contains(logLine, `"route":"/api/pvcs"`) {
		t.Fatalf("log missing route: %s", logLine)
	}
	if strings.Contains(logLine, "kubeconfig") || strings.Contains(logLine, "token") {
		t.Fatalf("log leaked sensitive value: %s", logLine)
	}
}

func TestWritePrometheusExposesCounters(t *testing.T) {
	t.Parallel()

	recorder := New(config.ObservabilityConfig{LogLevel: "error"}, nil)
	recorder.Metrics().HTTPRequests.Add(2)
	recorder.Metrics().CleanupDeleted.Add(1)
	response := httptest.NewRecorder()

	recorder.WritePrometheus(response)

	body := response.Body.String()
	if !strings.Contains(body, "viewer_http_requests_total 2") {
		t.Fatalf("metrics body missing HTTP count: %s", body)
	}
	if !strings.Contains(body, "viewer_cleanup_deleted_total 1") {
		t.Fatalf("metrics body missing cleanup count: %s", body)
	}
}
