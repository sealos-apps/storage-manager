package session

import (
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCleanupPurgesExpiredViewerSession(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_1",
		PodSessionID: "ps_1",
		ExpiresAt:    fixedNow().Add(-time.Second),
	})
	recorder := observability.MustNew(cfg.Observability, nil)
	cleanup := NewCleanupService(
		cfg,
		store,
		NewPodService(cfg, store, kube.New(fake.NewSimpleClientset()), recorder),
		recorder,
	)
	cleanup.now = fixedNow

	if err := cleanup.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if got := store.ListViewerSessionsByPod("ps_1", fixedNow()); len(got) != 0 {
		t.Fatalf("viewer sessions = %d", len(got))
	}
}

func TestCleanupClosesExpiredIdlePodSession(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	pod := viewerPodFixture("default", "viewer-ps_idle", "ps_idle")
	clientset := fake.NewSimpleClientset(
		pod,
		serviceFixture("default", "viewer-ps_idle"),
		ingressFixture("default", "viewer-ps_idle"),
	)
	pods := NewPodService(cfg, store, kube.New(clientset), observability.MustNew(cfg.Observability, nil))
	store.PutPodSession(&domain.PodSession{
		ID:          "ps_idle",
		Namespace:   "default",
		PodName:     "viewer-ps_idle",
		ServiceName: "viewer-ps_idle",
		PVCUID:      "uid",
		Status:      domain.PodStatusReady,
		ExpiresAt:   fixedNow().Add(-time.Second),
	})
	cleanup := NewCleanupService(cfg, store, pods, observability.MustNew(cfg.Observability, nil))
	cleanup.now = fixedNow

	if err := cleanup.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if _, ok := store.GetPodSession("ps_idle", fixedNow()); ok {
		t.Fatal("expired idle pod session remained in state")
	}
	if _, err := clientset.CoreV1().Pods("default").Get(t.Context(), "viewer-ps_idle", metav1.GetOptions{}); err == nil {
		t.Fatal("viewer pod was not deleted")
	}
}

func TestCleanupKeepsExpiredPodWithActiveViewer(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	pods := NewPodService(
		cfg,
		store,
		kube.New(fake.NewSimpleClientset(viewerPodFixture("default", "viewer-ps_active", "ps_active"))),
		observability.MustNew(cfg.Observability, nil),
	)
	store.PutPodSession(&domain.PodSession{
		ID:          "ps_active",
		Namespace:   "default",
		PodName:     "viewer-ps_active",
		ServiceName: "viewer-ps_active",
		PVCUID:      "uid",
		Status:      domain.PodStatusReady,
		ExpiresAt:   fixedNow().Add(-time.Second),
	})
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_active",
		PodSessionID: "ps_active",
		Status:       domain.ViewerStatusReady,
		ExpiresAt:    fixedNow().Add(time.Minute),
	})
	cleanup := NewCleanupService(cfg, store, pods, observability.MustNew(cfg.Observability, nil))
	cleanup.now = fixedNow

	if err := cleanup.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if _, ok := store.GetPodSession("ps_active", fixedNow()); !ok {
		t.Fatal("pod with active viewer was removed")
	}
}

func TestReconcileViewerPodsRecoversRecentPod(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "viewer-ps_recent",
			CreationTimestamp: metav1.NewTime(fixedNow().Add(-time.Minute)),
			Labels: map[string]string{
				labelComponent:    componentViewer,
				labelPVCName:      "data",
				labelPVCUID:       "uid",
				labelPodSessionID: "ps_recent",
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	service := NewPodService(
		cfg,
		store,
		kube.New(fake.NewSimpleClientset(pod)),
		observability.MustNew(cfg.Observability, nil),
	)
	service.now = fixedNow

	if err := service.ReconcileViewerPods(t.Context(), "default"); err != nil {
		t.Fatalf("ReconcileViewerPods() error = %v", err)
	}
	if _, ok := store.GetPodSession("ps_recent", fixedNow()); !ok {
		t.Fatal("recent pod was not recovered into state")
	}
}

func viewerPodFixture(namespace string, name string, podSessionID string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				labelComponent:    componentViewer,
				labelPVCName:      "data",
				labelPVCUID:       "uid",
				labelPodSessionID: podSessionID,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func serviceFixture(namespace string, name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func ingressFixture(namespace string, name string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}
