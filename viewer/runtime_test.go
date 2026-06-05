package viewer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"k8s.io/client-go/rest"
)

func TestManagementRESTConfigUsesConfiguredKubeconfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	kubeconfigPath := filepath.Join(dir, "management.kubeconfig.yaml")
	if err := os.WriteFile(kubeconfigPath, []byte(testKubeconfig), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	cfg := config.Default()
	cfg.Debug.Enabled = true
	cfg.Debug.ManagementKubeconfigPath = kubeconfigPath

	restConfig, err := managementRESTConfig(cfg)
	if err != nil {
		t.Fatalf("managementRESTConfig() error = %v", err)
	}
	if restConfig.Host != "https://127.0.0.1:6443" {
		t.Fatalf("host = %q", restConfig.Host)
	}
}

func TestStorageClassAdminKubeClientUsesBackendNamespaceImpersonation(t *testing.T) {
	if got := storageClassAdminUsername("backend-ns", "storageclass-admin"); got != "system:serviceaccount:backend-ns:storageclass-admin" {
		t.Fatalf("storageClassAdminUsername() = %q", got)
	}

	dir := t.TempDir()
	namespacePath := filepath.Join(dir, "namespace")
	if err := os.WriteFile(namespacePath, []byte("backend-ns\n"), 0o600); err != nil {
		t.Fatalf("write namespace: %v", err)
	}
	oldPath := serviceAccountNamespacePath
	serviceAccountNamespacePath = namespacePath
	t.Cleanup(func() {
		serviceAccountNamespacePath = oldPath
	})

	cfg := config.Default()
	cfg.Admin.StorageClassServiceAccountName = "storageclass-admin"
	client, err := storageClassAdminKubeClient(
		cfg,
		&rest.Config{Host: "https://127.0.0.1:6443"},
		observabilityTestRecorder(t),
	)
	if err != nil {
		t.Fatalf("storageClassAdminKubeClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("client = nil")
	}
}

func TestStorageClassAdminKubeClientUsesDebugManagementKubeconfigDirectly(t *testing.T) {
	oldPath := serviceAccountNamespacePath
	serviceAccountNamespacePath = filepath.Join(t.TempDir(), "missing-namespace")
	t.Cleanup(func() {
		serviceAccountNamespacePath = oldPath
	})

	var impersonateUser string
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		impersonateUser = req.Header.Get("Impersonate-User")
		authorization = req.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"StorageClassList","apiVersion":"storage.k8s.io/v1","items":[]}`))
	}))
	t.Cleanup(server.Close)

	cfg := config.Default()
	cfg.Debug.Enabled = true
	cfg.Debug.ManagementKubeconfigPath = "kubeconfig.management.yaml"
	client, err := storageClassAdminKubeClient(
		cfg,
		&rest.Config{Host: server.URL, BearerToken: "management-token"},
		observabilityTestRecorder(t),
	)
	if err != nil {
		t.Fatalf("storageClassAdminKubeClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("client = nil")
	}
	if _, err := client.ListStorageClasses(context.Background()); err != nil {
		t.Fatalf("ListStorageClasses() error = %v", err)
	}
	if impersonateUser != "" {
		t.Fatalf("Impersonate-User = %q", impersonateUser)
	}
	if authorization != "Bearer management-token" {
		t.Fatalf("Authorization = %q", authorization)
	}
}

func TestBackendNamespaceRejectsEmptyNamespace(t *testing.T) {
	dir := t.TempDir()
	namespacePath := filepath.Join(dir, "namespace")
	if err := os.WriteFile(namespacePath, []byte(" \n"), 0o600); err != nil {
		t.Fatalf("write namespace: %v", err)
	}
	oldPath := serviceAccountNamespacePath
	serviceAccountNamespacePath = namespacePath
	t.Cleanup(func() {
		serviceAccountNamespacePath = oldPath
	})

	_, err := backendNamespace()
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("backendNamespace() error = %v", err)
	}
}

func TestWrapKubernetesTransportInjectsTraceContext(t *testing.T) {
	t.Parallel()

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	cfg := &rest.Config{}
	wrapKubernetesTransport(cfg, provider)
	if cfg.WrapTransport == nil {
		t.Fatal("WrapTransport was not configured")
	}

	var traceparent string
	transport := cfg.WrapTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		traceparent = req.Header.Get("traceparent")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}))
	req, err := http.NewRequest(http.MethodGet, "https://kubernetes.example/api", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	ctx, span := provider.Tracer("test").Start(req.Context(), "parent")
	defer span.End()
	resp, err := transport.RoundTrip(req.WithContext(ctx))
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	_ = resp.Body.Close()

	if traceparent == "" {
		t.Fatal("traceparent header was not injected")
	}
}

func observabilityTestRecorder(t *testing.T) *observability.Recorder {
	t.Helper()

	cfg := config.Default().Observability
	cfg.Logs.Exporter = "discard"
	return observability.MustNew(cfg, nil)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
