package kube

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestObservedClientRecordsKubernetesRequest(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	recorder := observability.New(config.ObservabilityConfig{LogLevel: "debug"}, &out)
	client := WithObservability(New(fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "data",
		},
	})), recorder)

	if _, err := client.GetPVC(t.Context(), "default", "data"); err != nil {
		t.Fatalf("GetPVC() error = %v", err)
	}
	if recorder.Metrics().KubernetesRequests.Load() != 1 {
		t.Fatalf("kubernetes requests = %d", recorder.Metrics().KubernetesRequests.Load())
	}
	if recorder.Metrics().KubernetesErrors.Load() != 0 {
		t.Fatalf("kubernetes errors = %d", recorder.Metrics().KubernetesErrors.Load())
	}
	logs := out.String()
	if !strings.Contains(logs, `"span":"kubernetes.get"`) ||
		!strings.Contains(logs, `"resource":"persistentvolumeclaim"`) {
		t.Fatalf("missing kubernetes span fields: %s", logs)
	}
}

func TestObservedClientRecordsKubernetesError(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	recorder := observability.New(config.ObservabilityConfig{LogLevel: "debug"}, &out)
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor(
		"get",
		"persistentvolumeclaims",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("kube unavailable")
		},
	)
	client := WithObservability(New(clientset), recorder)

	if _, err := client.GetPVC(t.Context(), "default", "data"); err == nil {
		t.Fatal("GetPVC() error = nil")
	}
	if recorder.Metrics().KubernetesRequests.Load() != 1 {
		t.Fatalf("kubernetes requests = %d", recorder.Metrics().KubernetesRequests.Load())
	}
	if recorder.Metrics().KubernetesErrors.Load() != 1 {
		t.Fatalf("kubernetes errors = %d", recorder.Metrics().KubernetesErrors.Load())
	}
	if !strings.Contains(out.String(), "kube unavailable") {
		t.Fatalf("missing error in span log: %s", out.String())
	}
}
