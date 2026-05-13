package observability

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-stroage-manager/internal/config"
)

func TestObserveHTTPRecordsMetricsAndStructuredLog(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	recorder := New(config.ObservabilityConfig{LogLevel: "info"}, &out)
	recorder.ObserveHTTP(context.Background(), "GET", "/api/pvcs", 500, time.Millisecond)

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
