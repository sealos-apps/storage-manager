package accountquota

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
)

func TestClientStorageQuotaReturnsAvailableBytesAndQuantity(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotWorkspace string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotAuth = req.Header.Get("Authorization")
		var body map[string]string
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotWorkspace = body["workspace"]
		_, _ = w.Write([]byte(`{"quota":{"hard":{"requests.storage":"20Gi"},"used":{"requests.storage":"5Gi"}}}`))
	}))
	t.Cleanup(server.Close)
	client := NewClient(config.StorageQuotaConfig{
		AccountBaseURL: server.URL,
		QueryTimeout:   time.Second,
	}, server.Client(), nil)

	quota, err := client.StorageQuota(context.Background(), "ns-demo", "Bearer token")
	if err != nil {
		t.Fatalf("StorageQuota() error = %v", err)
	}

	if gotAuth != "Bearer token" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotWorkspace != "ns-demo" {
		t.Fatalf("workspace = %q", gotWorkspace)
	}
	if quota.AvailableBytes != 15*1024*1024*1024 {
		t.Fatalf("available bytes = %d", quota.AvailableBytes)
	}
	if quota.AvailableQuantity != "15Gi" {
		t.Fatalf("available quantity = %q", quota.AvailableQuantity)
	}
}

func TestStorageQuotaFromStatusClampsNegativeAvailableStorage(t *testing.T) {
	t.Parallel()

	quota, err := storageQuotaFromStatus(
		map[string]string{"requests.storage": "1Gi"},
		map[string]string{"requests.storage": "2Gi"},
	)
	if err != nil {
		t.Fatalf("storageQuotaFromStatus() error = %v", err)
	}
	if quota.AvailableBytes != 0 {
		t.Fatalf("available bytes = %d", quota.AvailableBytes)
	}
	if quota.AvailableQuantity != "0" {
		t.Fatalf("available quantity = %q", quota.AvailableQuantity)
	}
}
