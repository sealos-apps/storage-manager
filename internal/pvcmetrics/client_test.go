package pvcmetrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
)

func TestClientListPVCVolumeStatsParsesCompleteMetricsAndSampleTimes(t *testing.T) {
	t.Parallel()

	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/select/0/prometheus/api/v1/query" {
			t.Fatalf("unexpected path %q", req.URL.Path)
		}
		query := req.URL.Query().Get("query")
		queries = append(queries, query)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(query, "{__name__=~"):
			writePrometheusResponse(t, w, []map[string]any{
				metricResult("kubelet_volume_stats_used_bytes", "data", "100"),
				metricResult("kubelet_volume_stats_capacity_bytes", "data", "200"),
				metricResult("kubelet_volume_stats_available_bytes", "data", "80"),
				metricResult("kubelet_volume_stats_used_bytes", "partial", "25"),
				metricResult("kubelet_volume_stats_capacity_bytes", "partial", "50"),
			})
		case strings.HasPrefix(query, "timestamp("):
			writePrometheusResponse(t, w, []map[string]any{
				timestampResult("data", "1781084772.284"),
			})
		default:
			t.Fatalf("unexpected query %q", query)
		}
	}))
	defer server.Close()

	client := NewClient(config.PVCMetricsConfig{
		Enabled:           true,
		PrometheusBaseURL: server.URL + "/select/0/prometheus",
		QueryTimeout:      time.Second,
	}, server.Client(), nil)

	stats, err := client.ListPVCVolumeStats(context.Background(), "ns-admin")
	if err != nil {
		t.Fatalf("ListPVCVolumeStats() error = %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("stats len = %d, want 1: %#v", len(stats), stats)
	}
	got, ok := stats["data"]
	if !ok {
		t.Fatalf("stats missing data pvc: %#v", stats)
	}
	if got.Source != "kubelet" || got.Status != "ready" {
		t.Fatalf("status/source = %q/%q", got.Status, got.Source)
	}
	if got.UsedBytes != 100 || got.MetricCapacityBytes != 200 || got.AvailableBytes != 80 {
		t.Fatalf("stats bytes = %#v", got)
	}
	if got.SampleTime == nil || got.SampleTime.Unix() != 1781084772 {
		t.Fatalf("sample time = %#v", got.SampleTime)
	}
	if len(queries) != 2 {
		t.Fatalf("query count = %d, want 2: %#v", len(queries), queries)
	}
	if !strings.Contains(queries[0], `namespace="ns-admin"`) || !strings.Contains(queries[0], `job="kubelet"`) {
		t.Fatalf("metrics query did not include namespace/job: %q", queries[0])
	}
}

func TestClientListPVCVolumeStatsReturnsErrorForPrometheusFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer server.Close()

	client := NewClient(config.PVCMetricsConfig{
		Enabled:           true,
		PrometheusBaseURL: server.URL + "/select/0/prometheus",
		QueryTimeout:      time.Second,
	}, server.Client(), nil)

	_, err := client.ListPVCVolumeStats(context.Background(), "ns-admin")
	if err == nil || !strings.Contains(err.Error(), "returned 502") {
		t.Fatalf("error = %v, want status context", err)
	}
}

func metricResult(name string, pvc string, value string) map[string]any {
	return map[string]any{
		"metric": map[string]string{
			"__name__":              name,
			"namespace":             "ns-admin",
			"persistentvolumeclaim": pvc,
			"job":                   "kubelet",
			"node":                  "sealos-template",
			"instance":              "sealos-template",
		},
		"value": []any{float64(1781084772), value},
	}
}

func timestampResult(pvc string, value string) map[string]any {
	return map[string]any{
		"metric": map[string]string{
			"namespace":             "ns-admin",
			"persistentvolumeclaim": pvc,
			"job":                   "kubelet",
		},
		"value": []any{float64(1781084772), value},
	}
}

func writePrometheusResponse(t *testing.T, w http.ResponseWriter, result []map[string]any) {
	t.Helper()
	body := map[string]any{
		"status": "success",
		"data": map[string]any{
			"result": result,
		},
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encoding response: %v", err)
	}
}
