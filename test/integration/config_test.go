//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-stroage-manager/internal/config"
	"github.com/nixieboluo/sealos-stroage-manager/internal/kube"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

func TestIntegrationOwnerReferencesGarbageCollectViewerAttachments(t *testing.T) {
	root := repoRoot(t)
	cfg := loadIntegrationConfig(t, root)
	managementPath := cfg.Integration.ManagementKubeconfigPath
	if managementPath == "" {
		managementPath = cfg.Kubernetes.ManagementKubeconfigPath
	}
	client := integrationClient(t, root, managementPath)
	namespace := cfg.Integration.Namespace
	name := integrationResourceName(t)
	ctx := context.Background()

	pod, err := client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				"integration-test": "owner-reference-gc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "pause",
					Image: "registry.k8s.io/pause:3.9",
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create owner pod: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().Pods(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	owner := metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       pod.Name,
		UID:        pod.UID,
	}
	if _, err := client.CoreV1().ConfigMaps(namespace).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       namespace,
			Name:            name,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Data: map[string]string{"filebrowser-auth-hook.sh": "echo hook.action=block\n"},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create owned configmap: %v", err)
	}
	if _, err := client.CoreV1().Services(namespace).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       namespace,
			Name:            name,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"integration-test": "owner-reference-gc"},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(8080),
				},
			},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create owned service: %v", err)
	}
	pathType := networkingv1.PathTypePrefix
	if _, err := client.NetworkingV1().Ingresses(namespace).Create(ctx, &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       namespace,
			Name:            name,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: name + ".example.test",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: name,
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create owned ingress: %v", err)
	}

	if err := client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete owner pod: %v", err)
	}
	waitForNotFound(t, "owned configmap", func(ctx context.Context) error {
		_, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	})
	waitForNotFound(t, "owned service", func(ctx context.Context) error {
		_, err := client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	})
	waitForNotFound(t, "owned ingress", func(ctx context.Context) error {
		_, err := client.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	})
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

func integrationResourceName(t *testing.T) string {
	t.Helper()

	var randomBytes [4]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	return "ownerref-" + hex.EncodeToString(randomBytes[:])
}

func waitForNotFound(t *testing.T, resource string, get func(context.Context) error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := get(ctx)
		if apierrors.IsNotFound(err) {
			return
		}
		if err != nil {
			t.Fatalf("get %s: %v", resource, err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s still exists after owner pod deletion", resource)
		case <-ticker.C:
		}
	}
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
