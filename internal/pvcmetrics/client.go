package pvcmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
)

const (
	sourceKubelet = "kubelet"
	statusReady   = "ready"
)

type Reader interface {
	ListPVCVolumeStats(ctx context.Context, namespace string) (map[string]domain.PVCVolumeStats, error)
}

type Client struct {
	baseURL      string
	http         *http.Client
	queryTimeout time.Duration
	recorder     *observability.Recorder
}

type queryResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []queryResult `json:"result"`
	} `json:"data"`
}

type queryResult struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
}

type partialStats struct {
	stats domain.PVCVolumeStats
	seen  map[string]bool
}

func NewClient(cfg config.PVCMetricsConfig, httpClient *http.Client, recorder *observability.Recorder) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:      strings.TrimRight(strings.TrimSpace(cfg.PrometheusBaseURL), "/"),
		http:         httpClient,
		queryTimeout: cfg.QueryTimeout,
		recorder:     recorder,
	}
}

func (c *Client) ListPVCVolumeStats(ctx context.Context, namespace string) (stats map[string]domain.PVCVolumeStats, err error) {
	ctx, finish := c.trace(ctx, "pvc_metrics.list_volume_stats", slog.String("namespace", namespace))
	defer func() {
		finish(err)
	}()

	timeout := c.queryTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	results, err := c.query(ctx, pvcMetricsQuery(namespace))
	if err != nil {
		return nil, err
	}
	timestamps, err := c.query(ctx, pvcTimestampQuery(namespace))
	if err != nil {
		return nil, err
	}
	return completeStats(results, timestamps), nil
}

func (c *Client) query(ctx context.Context, query string) ([]queryResult, error) {
	endpoint, err := url.Parse(c.baseURL + "/api/v1/query")
	if err != nil {
		return nil, fmt.Errorf("parsing pvc metrics query url: %w", err)
	}
	values := endpoint.Query()
	values.Set("query", query)
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building pvc metrics query request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling pvc metrics query: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("pvc metrics query returned %d", resp.StatusCode)
	}
	var out queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding pvc metrics query response: %w", err)
	}
	if out.Status != "success" {
		if strings.TrimSpace(out.Error) != "" {
			return nil, fmt.Errorf("pvc metrics query failed: %s", out.Error)
		}
		return nil, fmt.Errorf("pvc metrics query failed with status %q", out.Status)
	}
	return out.Data.Result, nil
}

func completeStats(results []queryResult, timestamps []queryResult) map[string]domain.PVCVolumeStats {
	byPVC := map[string]*partialStats{}
	for _, result := range results {
		pvc := strings.TrimSpace(result.Metric["persistentvolumeclaim"])
		if pvc == "" {
			continue
		}
		value, ok := int64Value(result.Value)
		if !ok {
			continue
		}
		entry := byPVC[pvc]
		if entry == nil {
			entry = &partialStats{
				stats: domain.PVCVolumeStats{
					Source: sourceKubelet,
					Status: statusReady,
				},
				seen: map[string]bool{},
			}
			byPVC[pvc] = entry
		}
		switch result.Metric["__name__"] {
		case "kubelet_volume_stats_used_bytes":
			entry.stats.UsedBytes = value
			entry.seen["used"] = true
		case "kubelet_volume_stats_capacity_bytes":
			entry.stats.MetricCapacityBytes = value
			entry.seen["capacity"] = true
		case "kubelet_volume_stats_available_bytes":
			entry.stats.AvailableBytes = value
			entry.seen["available"] = true
		}
	}
	for _, result := range timestamps {
		pvc := strings.TrimSpace(result.Metric["persistentvolumeclaim"])
		entry := byPVC[pvc]
		if entry == nil {
			continue
		}
		if sampleTime, ok := unixTimeValue(result.Value); ok {
			entry.stats.SampleTime = &sampleTime
		}
	}
	complete := map[string]domain.PVCVolumeStats{}
	for pvc, entry := range byPVC {
		if entry.seen["used"] && entry.seen["capacity"] && entry.seen["available"] {
			complete[pvc] = entry.stats
		}
	}
	return complete
}

func pvcMetricsQuery(namespace string) string {
	return fmt.Sprintf(
		`{__name__=~"kubelet_volume_stats_(used|capacity|available)_bytes",namespace=%q,job="kubelet"}`,
		namespace,
	)
}

func pvcTimestampQuery(namespace string) string {
	return fmt.Sprintf(
		`timestamp(kubelet_volume_stats_used_bytes{namespace=%q,job="kubelet"})`,
		namespace,
	)
}

func int64Value(value []any) (int64, bool) {
	if len(value) < 2 {
		return 0, false
	}
	text, ok := value[1].(string)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func unixTimeValue(value []any) (time.Time, bool) {
	if len(value) < 2 {
		return time.Time{}, false
	}
	text, ok := value[1].(string)
	if !ok {
		return time.Time{}, false
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return time.Time{}, false
	}
	seconds, fraction := math.Modf(parsed)
	return time.Unix(int64(seconds), int64(fraction*1e9)).UTC(), true
}

func (c *Client) trace(ctx context.Context, name string, attrs ...slog.Attr) (context.Context, func(error)) {
	if c.recorder == nil {
		return ctx, func(error) {}
	}
	return c.recorder.TraceOperation(ctx, name, attrs...)
}
