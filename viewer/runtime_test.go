package viewer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
)

func TestManagementRESTConfigUsesConfiguredKubeconfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	kubeconfigPath := filepath.Join(dir, "management.kubeconfig.yaml")
	if err := os.WriteFile(kubeconfigPath, []byte(testKubeconfig), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	cfg := config.Default()
	cfg.Kubernetes.ManagementKubeconfigPath = kubeconfigPath

	restConfig, err := managementRESTConfig(cfg)
	if err != nil {
		t.Fatalf("managementRESTConfig() error = %v", err)
	}
	if restConfig.Host != "https://127.0.0.1:6443" {
		t.Fatalf("host = %q", restConfig.Host)
	}
}
