//go:build integration

package integration

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/nixieboluo/sealos-stroage-manager/internal/config"
	"github.com/nixieboluo/sealos-stroage-manager/internal/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var configPath = flag.String("config", config.DefaultPath, "viewer backend config path")

func TestIntegrationKubeconfigCanListPVCs(t *testing.T) {
	root := repoRoot(t)
	cfg := loadIntegrationConfig(t, root)
	userClient := integrationClient(t, root, cfg.Integration.KubeconfigPath)
	client := kube.New(userClient)
	if _, err := client.ListPVCs(context.Background(), cfg.Integration.Namespace); err != nil {
		t.Fatalf("list pvcs in %q: %v", cfg.Integration.Namespace, err)
	}
}

func TestIntegrationUserAndManagementKubeconfigsResolveSameNamespace(t *testing.T) {
	root := repoRoot(t)
	cfg := loadIntegrationConfig(t, root)
	userClient := integrationClient(t, root, cfg.Integration.KubeconfigPath)
	managementPath := cfg.Integration.ManagementKubeconfigPath
	if managementPath == "" {
		managementPath = cfg.Kubernetes.ManagementKubeconfigPath
	}
	managementClient := integrationClient(t, root, managementPath)

	userNamespace, err := userClient.CoreV1().Namespaces().Get(
		context.Background(),
		cfg.Integration.Namespace,
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("user kubeconfig get namespace %q: %v", cfg.Integration.Namespace, err)
	}
	managementNamespace, err := managementClient.CoreV1().Namespaces().Get(
		context.Background(),
		cfg.Integration.Namespace,
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("management kubeconfig get namespace %q: %v", cfg.Integration.Namespace, err)
	}
	if userNamespace.UID != managementNamespace.UID {
		t.Fatalf("namespace UID mismatch: user=%q management=%q", userNamespace.UID, managementNamespace.UID)
	}
}

func loadIntegrationConfig(t *testing.T, root string) config.Config {
	t.Helper()
	cfgPath := *configPath
	if !filepath.IsAbs(cfgPath) {
		cfgPath = filepath.Join(root, cfgPath)
	}
	cfg, err := config.LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Integration.KubeconfigPath == "" {
		t.Skip("integration.kubeconfig_path is empty")
	}
	return cfg
}

func integrationClient(t *testing.T, root string, kubeconfigPath string) kubernetes.Interface {
	t.Helper()
	if kubeconfigPath == "" {
		t.Skip("kubeconfig path is empty")
	}
	if !filepath.IsAbs(kubeconfigPath) {
		kubeconfigPath = filepath.Join(root, kubeconfigPath)
	}
	if _, err := os.Stat(kubeconfigPath); err != nil {
		t.Skipf("integration kubeconfig %q unavailable: %v", kubeconfigPath, err)
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		t.Fatalf("build rest config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("new kubernetes client: %v", err)
	}
	return clientset
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root")
		}
		dir = parent
	}
}
