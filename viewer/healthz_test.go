package viewer

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func resetRuntimeStateForTest(t *testing.T, loader func(string) (*Runtime, error)) {
	t.Helper()

	originalDefaultHandler := defaultHandler
	originalInitErr := runtimeInitErr
	originalLoader := loadRuntime

	runtimeOnce = sync.Once{}
	defaultHandler = nil
	runtimeInitErr = nil
	loadRuntime = loader

	t.Cleanup(func() {
		runtimeOnce = sync.Once{}
		defaultHandler = originalDefaultHandler
		runtimeInitErr = originalInitErr
		loadRuntime = originalLoader
	})
}

func TestHealthzReturnsOkWhenRuntimeInitializes(t *testing.T) {
	resetRuntimeStateForTest(t, func(string) (*Runtime, error) {
		return &Runtime{Handler: &Handler{}}, nil
	})

	resp, err := Healthz(context.Background())
	if err != nil {
		t.Fatalf("Healthz() error = %v", err)
	}

	if resp == nil {
		t.Fatal("Healthz() response is nil")
	}
	if resp.Service != "storage-manager-viewer" {
		t.Fatalf("Healthz() service = %q, want storage-manager-viewer", resp.Service)
	}
	if resp.Status != "ok" {
		t.Fatalf("Healthz() status = %q, want ok", resp.Status)
	}
}

func TestRuntimeHealthReturnsInitializationError(t *testing.T) {
	resetRuntimeStateForTest(t, func(string) (*Runtime, error) {
		return nil, errors.New("config load failed")
	})

	if err := runtimeHealth(); err == nil || err.Error() != "config load failed" {
		t.Fatalf("runtimeHealth() = %v, want config load failed", err)
	}
}
