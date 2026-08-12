package accountquota

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"k8s.io/apimachinery/pkg/api/resource"
)

const resourceStorageRequests = "requests.storage"

type StorageQuota struct {
	AvailableBytes    int64
	AvailableQuantity string
	LimitBytes        int64
	LimitQuantity     string
	UsedBytes         int64
	UsedQuantity      string
}

type Client struct {
	baseURL  string
	http     *http.Client
	recorder *observability.Recorder
}

type upstreamQuotaResponse struct {
	Quota struct {
		Hard map[string]quotaQuantity `json:"hard"`
		Used map[string]quotaQuantity `json:"used"`
	} `json:"quota"`
}

// quotaQuantity accepts both Kubernetes-style quantity strings and the byte
// values returned by account-service versions that serialize quantities as JSON numbers.
type quotaQuantity string

func (q *quotaQuantity) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return fmt.Errorf("quota quantity is empty")
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*q = quotaQuantity(value)
		return nil
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return fmt.Errorf("expected string or number, got %s: %w", string(data), err)
	}
	if number.String() == "" {
		return fmt.Errorf("quota quantity is empty")
	}
	*q = quotaQuantity(number.String())
	return nil
}

func quantityStrings(values map[string]quotaQuantity) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = string(value)
	}
	return result
}

func NewClient(
	cfg config.StorageQuotaConfig,
	httpClient *http.Client,
	recorder *observability.Recorder,
) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.QueryTimeout}
	}
	return &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.AccountBaseURL), "/"),
		http:     httpClient,
		recorder: recorder,
	}
}

func (c *Client) StorageQuota(ctx context.Context, workspace string, authorization string) (quota StorageQuota, err error) {
	if c == nil || c.baseURL == "" {
		return StorageQuota{}, fmt.Errorf("account quota client is not configured")
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return StorageQuota{}, fmt.Errorf("workspace is required")
	}
	ctx, finish := c.trace(ctx, "account_quota.storage_quota", slog.String("namespace", workspace))
	defer func() {
		finish(err)
	}()

	body, err := json.Marshal(map[string]string{"workspace": workspace})
	if err != nil {
		return StorageQuota{}, fmt.Errorf("encoding workspace quota request: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/account/v1alpha1/workspace/get-resource-quota",
		bytes.NewReader(body),
	)
	if err != nil {
		return StorageQuota{}, fmt.Errorf("building workspace quota request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(authorization) != "" {
		req.Header.Set("Authorization", authorization)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return StorageQuota{}, fmt.Errorf("requesting workspace quota: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return StorageQuota{}, fmt.Errorf("workspace quota status %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	var data upstreamQuotaResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return StorageQuota{}, fmt.Errorf("decoding workspace quota response: %w", err)
	}
	return storageQuotaFromStatus(quantityStrings(data.Quota.Hard), quantityStrings(data.Quota.Used))
}

func (c *Client) trace(ctx context.Context, name string, attrs ...slog.Attr) (context.Context, func(error)) {
	if c.recorder == nil {
		return ctx, func(error) {}
	}
	return c.recorder.TraceOperation(ctx, name, attrs...)
}

func storageQuotaFromStatus(hard map[string]string, used map[string]string) (StorageQuota, error) {
	limit, err := parseQuantity(hard[resourceStorageRequests])
	if err != nil {
		return StorageQuota{}, fmt.Errorf("parsing storage quota limit: %w", err)
	}
	usedQuantity, err := parseQuantity(used[resourceStorageRequests])
	if err != nil {
		return StorageQuota{}, fmt.Errorf("parsing storage quota used: %w", err)
	}
	availableBytes := limit.Value() - usedQuantity.Value()
	if availableBytes < 0 {
		availableBytes = 0
	}
	available := *resource.NewQuantity(availableBytes, resource.BinarySI)
	return StorageQuota{
		AvailableBytes:    availableBytes,
		AvailableQuantity: available.String(),
		LimitBytes:        limit.Value(),
		LimitQuantity:     limit.String(),
		UsedBytes:         usedQuantity.Value(),
		UsedQuantity:      usedQuantity.String(),
	}, nil
}

func parseQuantity(value string) (resource.Quantity, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "0"
	}
	return resource.ParseQuantity(value)
}

func BinaryQuantity(bytes int64) string {
	if bytes <= 0 {
		return "0"
	}
	quantity := *resource.NewQuantity(bytes, resource.BinarySI)
	return quantity.String()
}
