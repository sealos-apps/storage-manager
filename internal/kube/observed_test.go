package kube

import (
	"errors"
	"net/http/httptest"
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
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	recorder := observability.MustNew(testObservability(), nil)
	client := WithObservability(New(fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "data",
		},
	})), recorder)

	if _, err := client.GetPVC(t.Context(), "default", "data"); err != nil {
		t.Fatalf("GetPVC() error = %v", err)
	}
	metrics := prometheusText(t, recorder)
	if !strings.Contains(metrics, `viewer_kubernetes_operation_requests_total{Operation="get",Resource="persistentvolumeclaim",Result="success"} 1`) {
		t.Fatalf("missing kubernetes request metric: %s", metrics)
	}
}

func TestObservedClientRecordsPVCMutation(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	recorder := observability.MustNew(testObservability(), nil)
	client := WithObservability(New(fake.NewSimpleClientset()), recorder)

	if _, err := client.CreatePVC(t.Context(), &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "data",
		},
	}); err != nil {
		t.Fatalf("CreatePVC() error = %v", err)
	}
	metrics := prometheusText(t, recorder)
	if !strings.Contains(metrics, `viewer_kubernetes_operation_requests_total{Operation="create",Resource="persistentvolumeclaim",Result="success"} 1`) {
		t.Fatalf("missing pvc create metric: %s", metrics)
	}
}

func TestObservedClientRecordsNamespaceList(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	recorder := observability.MustNew(testObservability(), nil)
	client := WithObservability(New(fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)), recorder)

	if _, err := client.ListNamespaces(t.Context()); err != nil {
		t.Fatalf("ListNamespaces() error = %v", err)
	}
	metrics := prometheusText(t, recorder)
	if !strings.Contains(metrics, `viewer_kubernetes_operation_requests_total{Operation="list",Resource="namespaces",Result="success"} 1`) {
		t.Fatalf("missing namespace list metric: %s", metrics)
	}
}

func TestObservedClientRecordsKubernetesError(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	recorder := observability.MustNew(testObservability(), nil)
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
	metrics := prometheusText(t, recorder)
	if !strings.Contains(metrics, `viewer_kubernetes_operation_errors_total{Operation="get",Resource="persistentvolumeclaim"} 1`) {
		t.Fatalf("missing kubernetes error metric: %s", metrics)
	}
}

func testObservability() config.ObservabilityConfig {
	cfg := config.Default().Observability
	cfg.Logs.Exporter = "discard"
	cfg.Logs.Level = "error"
	return cfg
}

func prometheusText(t *testing.T, recorder *observability.Recorder) string {
	t.Helper()

	response := httptest.NewRecorder()
	recorder.WritePrometheus(response, httptest.NewRequest("GET", "/metrics", nil))
	return response.Body.String()
}
