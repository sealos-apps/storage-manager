//go:build integration

package integration

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/nixieboluo/sealos-stroage-manager/internal/config"
	"github.com/nixieboluo/sealos-stroage-manager/internal/kube"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var configPath = flag.String("config", config.DefaultPath, "viewer backend config path")

func TestIntegrationKubeconfigCanListPVCs(t *testing.T) {
	cfg, err := config.LoadFile(*configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Integration.KubeconfigPath == "" {
		t.Skip("integration.kubeconfig_path is empty")
	}
	if _, err := os.Stat(cfg.Integration.KubeconfigPath); err != nil {
		t.Skipf("integration kubeconfig unavailable: %v", err)
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", cfg.Integration.KubeconfigPath)
	if err != nil {
		t.Fatalf("build rest config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("new kubernetes client: %v", err)
	}
	client := kube.New(clientset)
	if _, err := client.ListPVCs(context.Background(), cfg.Integration.Namespace); err != nil {
		t.Fatalf("list pvcs in %q: %v", cfg.Integration.Namespace, err)
	}
}
