//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func TestIntegrationConfigLoadsFromCONFIG(t *testing.T) {
	root := repoRoot(t)
	cfg := loadIntegrationConfig(t, root)
	if !cfg.Debug.Enabled {
		t.Fatal("debug.enabled = false")
	}
	if strings.TrimSpace(cfg.Debug.ManagementKubeconfigPath) == "" {
		t.Fatal("debug.management_kubeconfig_path is empty")
	}
}

func TestIntegrationKubeconfigCanListPVCs(t *testing.T) {
	root := repoRoot(t)
	cfg := loadIntegrationConfig(t, root)
	managementClient := integrationClient(t, root, cfg.Debug.ManagementKubeconfigPath)
	namespace := integrationNamespace(t, root, cfg)
	client := kube.New(managementClient)
	if _, err := client.ListPVCs(t.Context(), namespace); err != nil {
		t.Fatalf("list pvcs in %q: %v", namespace, err)
	}
}

func TestIntegrationListStorageClasses(t *testing.T) {
	root := repoRoot(t)
	cfg := loadIntegrationConfig(t, root)
	managementClient := integrationClient(t, root, cfg.Debug.ManagementKubeconfigPath)
	client := kube.New(managementClient)
	if _, err := client.ListStorageClasses(t.Context()); err != nil {
		t.Fatalf("list storageclasses: %v", err)
	}
}

func TestIntegrationManagementKubeconfigResolvesNamespace(t *testing.T) {
	root := repoRoot(t)
	cfg := loadIntegrationConfig(t, root)
	managementClient := integrationClient(t, root, cfg.Debug.ManagementKubeconfigPath)
	namespace := integrationNamespace(t, root, cfg)

	managementNamespace, err := managementClient.CoreV1().Namespaces().Get(t.Context(), namespace, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("management kubeconfig get namespace %q: %v", namespace, err)
	}
	if string(managementNamespace.UID) == "" {
		t.Fatalf("namespace %q has empty UID", namespace)
	}
}

func TestIntegrationCreateExpandDeletePVC(t *testing.T) {
	root := repoRoot(t)
	cfg := loadIntegrationConfig(t, root)
	managementClient := integrationClient(t, root, cfg.Debug.ManagementKubeconfigPath)
	namespace := integrationNamespace(t, root, cfg)
	name := integrationResourceName(t, "ssm-it-pvc")
	client := kube.New(managementClient)
	ctx := t.Context()

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 30*time.Second)
		defer cancel()
		_ = managementClient.CoreV1().PersistentVolumeClaims(namespace).Delete(cleanupCtx, name, metav1.DeleteOptions{})
	})

	created, err := client.CreatePVC(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "sealos-storage-manager",
				"integration-test":             "create-expand-delete-pvc",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	})
	if err != nil {
		t.Fatalf("create pvc: %v", err)
	}
	if created.Name != name {
		t.Fatalf("created pvc name = %q", created.Name)
	}

	expanded, err := client.UpdatePVCStorageRequest(ctx, namespace, name, resource.MustParse("2Gi"))
	if err != nil {
		t.Fatalf("expand pvc: %v", err)
	}
	if expanded.Spec.Resources.Requests.Storage().String() != "2Gi" {
		t.Fatalf("expanded storage = %s", expanded.Spec.Resources.Requests.Storage().String())
	}

	if err := client.DeletePVC(ctx, namespace, name); err != nil {
		t.Fatalf("delete pvc: %v", err)
	}
	waitForNotFound(t, "pvc", func(ctx context.Context) error {
		_, err := managementClient.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	})
}

func TestIntegrationOwnerReferencesGarbageCollectViewerAttachments(t *testing.T) {
	root := repoRoot(t)
	cfg := loadIntegrationConfig(t, root)
	client := integrationClient(t, root, cfg.Debug.ManagementKubeconfigPath)
	namespace := integrationNamespace(t, root, cfg)
	name := integrationResourceName(t, "ownerref")
	ctx := t.Context()

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
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		_ = client.CoreV1().Pods(namespace).Delete(cleanupCtx, name, metav1.DeleteOptions{})
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
	cfg, err := config.LoadFile("")
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file or directory") {
			t.Skipf("integration config unavailable: %v", err)
		}
		t.Fatalf("load config from %s: %v", config.EnvPath, err)
	}
	if !cfg.Debug.Enabled {
		t.Skip("debug.enabled is false")
	}
	if cfg.Debug.ManagementKubeconfigPath == "" {
		t.Skip("debug.management_kubeconfig_path is empty")
	}
	if _, ok := os.LookupEnv(config.EnvPath); !ok && !filepath.IsAbs(cfg.Server.ConfigPath) {
		cfg.Server.ConfigPath = filepath.Join(root, cfg.Server.ConfigPath)
	}
	return cfg
}

func integrationClient(t *testing.T, root string, kubeconfigPath string) kubernetes.Interface {
	t.Helper()
	path := resolvePath(root, kubeconfigPath)
	if path == "" {
		t.Skip("kubeconfig path is empty")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("integration kubeconfig %q unavailable: %v", path, err)
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("build rest config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("new kubernetes client: %v", err)
	}
	return clientset
}

func integrationNamespace(t *testing.T, root string, cfg config.Config) string {
	t.Helper()
	if namespace := strings.TrimSpace(cfg.Debug.ForcedNamespace); namespace != "" {
		return namespace
	}
	path := resolvePath(root, cfg.Debug.ManagementKubeconfigPath)
	configAccess := clientcmd.NewDefaultPathOptions()
	configAccess.LoadingRules.ExplicitPath = path
	rawConfig, err := configAccess.GetStartingConfig()
	if err != nil {
		t.Skipf("read management kubeconfig %q: %v", path, err)
	}
	contextName := rawConfig.CurrentContext
	if contextName == "" {
		return "default"
	}
	context := rawConfig.Contexts[contextName]
	if context == nil || strings.TrimSpace(context.Namespace) == "" {
		return "default"
	}
	return strings.TrimSpace(context.Namespace)
}

func resolvePath(root string, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func integrationResourceName(t *testing.T, prefix string) string {
	t.Helper()

	var randomBytes [4]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	return prefix + "-" + hex.EncodeToString(randomBytes[:])
}

func waitForNotFound(t *testing.T, resource string, get func(context.Context) error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
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
			t.Fatalf("%s still exists after deletion", resource)
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
