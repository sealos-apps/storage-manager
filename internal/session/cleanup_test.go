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
		ID:             "ps_idle",
		Namespace:      "default",
		PodName:        "viewer-ps_idle",
		ServiceName:    "viewer-ps_idle",
		PVCUID:         "uid",
		RuntimeVersion: pods.runtimeVersion,
		Status:         domain.PodStatusReady,
		ExpiresAt:      fixedNow().Add(-time.Second),
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
	pods.now = fixedNow
	store.PutPodSession(&domain.PodSession{
		ID:             "ps_active",
		Namespace:      "default",
		PodName:        "viewer-ps_active",
		ServiceName:    "viewer-ps_active",
		PVCUID:         "uid",
		RuntimeVersion: pods.runtimeVersion,
		Status:         domain.PodStatusReady,
		ExpiresAt:      fixedNow().Add(-time.Second),
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
	pod, err := pods.client.GetPod(t.Context(), "default", "viewer-ps_active")
	if err != nil {
		t.Fatalf("GetPod() active viewer pod error = %v", err)
	}
	if pod.Annotations[annotationKeepaliveUntil] != fixedNow().Add(cfg.Sessions.PodKeepaliveGrace).Format(time.RFC3339Nano) {
		t.Fatalf("active viewer keepalive annotation = %q", pod.Annotations[annotationKeepaliveUntil])
	}
}

func TestCleanupDeletesOrphanViewerPodsAcrossAllNamespaces(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	version := runtimeVersion(cfg)
	validPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "ns-a",
			Name:              "viewer-ps-valid",
			CreationTimestamp: metav1.NewTime(fixedNow().Add(-time.Minute)),
			Labels: map[string]string{
				labelComponent:      componentViewer,
				labelPVCName:        "data-a",
				labelPVCUID:         "uid-a",
				labelPodSessionID:   "ps_valid",
				labelRuntimeVersion: version,
			},
			Annotations: map[string]string{
				annotationAccessMode:     domain.AccessModeReadWriteMany,
				annotationCreatedAt:      fixedNow().Add(-time.Minute).Format(time.RFC3339Nano),
				annotationKeepaliveUntil: fixedNow().Add(time.Minute).Format(time.RFC3339Nano),
				annotationLastActiveAt:   fixedNow().Add(-time.Second).Format(time.RFC3339Nano),
				annotationMode:           domain.ModeReadWrite,
				annotationRuntimeVersion: version,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "ns-b",
			Name:              "viewer-ps-old",
			CreationTimestamp: metav1.NewTime(fixedNow().Add(-cfg.Sessions.OrphanGrace - time.Second)),
			Labels: map[string]string{
				labelComponent:      componentViewer,
				labelPVCName:        "data-b",
				labelPVCUID:         "uid-b",
				labelPodSessionID:   "ps_old",
				labelRuntimeVersion: "old-version",
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	clientset := fake.NewSimpleClientset(validPod, oldPod)
	recorder := observability.MustNew(cfg.Observability, nil)
	pods := NewPodService(cfg, store, kube.New(clientset), recorder)
	pods.now = fixedNow
	cleanup := NewCleanupService(cfg, store, pods, recorder)
	cleanup.now = fixedNow

	if err := cleanup.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if _, ok := store.GetPodSession("ps_valid", fixedNow()); ok {
		t.Fatal("cleanup recovered orphan viewer pod into state")
	}
	if _, err := clientset.CoreV1().Pods("ns-a").Get(t.Context(), "viewer-ps-valid", metav1.GetOptions{}); err == nil {
		t.Fatal("cleanup did not delete orphan viewer pod from another namespace")
	}
	if _, err := clientset.CoreV1().Pods("ns-b").Get(t.Context(), "viewer-ps-old", metav1.GetOptions{}); err == nil {
		t.Fatal("cleanup did not delete old orphan viewer pod from another namespace")
	}
}

func TestCleanupDeletesExpiredAnnotatedViewerPodAfterRestart(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	version := runtimeVersion(cfg)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "viewer-ps-expired",
			CreationTimestamp: metav1.NewTime(fixedNow().Add(-time.Minute)),
			Labels: map[string]string{
				labelComponent:      componentViewer,
				labelPVCName:        "data",
				labelPVCUID:         "uid",
				labelPodSessionID:   "ps_expired",
				labelRuntimeVersion: version,
			},
			Annotations: map[string]string{
				annotationAccessMode:     domain.AccessModeReadWriteMany,
				annotationCreatedAt:      fixedNow().Add(-time.Minute).Format(time.RFC3339Nano),
				annotationKeepaliveUntil: fixedNow().Add(-time.Second).Format(time.RFC3339Nano),
				annotationLastActiveAt:   fixedNow().Add(-time.Minute).Format(time.RFC3339Nano),
				annotationMode:           domain.ModeReadWrite,
				annotationRuntimeVersion: version,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	clientset := fake.NewSimpleClientset(pod)
	recorder := observability.MustNew(cfg.Observability, nil)
	pods := NewPodService(cfg, store, kube.New(clientset), recorder)
	pods.now = fixedNow
	cleanup := NewCleanupService(cfg, store, pods, recorder)
	cleanup.now = fixedNow

	if err := cleanup.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if _, ok := store.GetPodSessionIncludingExpired("ps_expired"); ok {
		t.Fatal("expired annotated pod was recovered into state")
	}
	if _, err := clientset.CoreV1().Pods("default").Get(t.Context(), "viewer-ps-expired", metav1.GetOptions{}); err == nil {
		t.Fatal("expired annotated viewer pod was not deleted")
	}
}

func TestReconcileViewerPodsDeletesRecentOrphanPod(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	version := runtimeVersion(cfg)
	store := state.New(cfg.Cache)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "viewer-ps_recent",
			CreationTimestamp: metav1.NewTime(fixedNow().Add(-time.Minute)),
			Labels: map[string]string{
				labelComponent:      componentViewer,
				labelPVCName:        "data",
				labelPVCUID:         "uid",
				labelPodSessionID:   "ps_recent",
				labelRuntimeVersion: version,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	clientset := fake.NewSimpleClientset(pod)
	service := NewPodService(
		cfg,
		store,
		kube.New(clientset),
		observability.MustNew(cfg.Observability, nil),
	)
	service.now = fixedNow

	if err := service.ReconcileViewerPods(t.Context(), "default"); err != nil {
		t.Fatalf("ReconcileViewerPods() error = %v", err)
	}
	if _, ok := store.GetPodSession("ps_recent", fixedNow()); ok {
		t.Fatal("recent orphan pod was recovered into state")
	}
	if _, err := clientset.CoreV1().Pods("default").Get(t.Context(), "viewer-ps_recent", metav1.GetOptions{}); err == nil {
		t.Fatal("recent orphan pod was not deleted")
	}
}

func TestReconcileViewerPodsDeletesUnannotatedPodPastRecoveryGrace(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	version := runtimeVersion(cfg)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "viewer-ps-stale",
			CreationTimestamp: metav1.NewTime(fixedNow().Add(-cfg.Sessions.RecoveryGrace - time.Second)),
			Labels: map[string]string{
				labelComponent:      componentViewer,
				labelPVCName:        "data",
				labelPVCUID:         "uid",
				labelPodSessionID:   "ps_stale",
				labelRuntimeVersion: version,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	clientset := fake.NewSimpleClientset(pod)
	service := NewPodService(
		cfg,
		store,
		kube.New(clientset),
		observability.MustNew(cfg.Observability, nil),
	)
	service.now = fixedNow

	if err := service.ReconcileViewerPods(t.Context(), "default"); err != nil {
		t.Fatalf("ReconcileViewerPods() error = %v", err)
	}
	if _, ok := store.GetPodSessionIncludingExpired("ps_stale"); ok {
		t.Fatal("stale unannotated pod was recovered into state")
	}
	if _, err := clientset.CoreV1().Pods("default").Get(t.Context(), "viewer-ps-stale", metav1.GetOptions{}); err == nil {
		t.Fatal("stale unannotated pod was not deleted")
	}
}

func TestReconcileViewerPodsSyncsExpiredStoredSessionWithoutPurging(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	version := runtimeVersion(cfg)
	store.PutPodSession(&domain.PodSession{
		ID:             "ps_expired",
		Namespace:      "default",
		PVCName:        "data",
		PVCUID:         "uid",
		PodName:        "viewer-ps-expired",
		ServiceName:    "viewer-ps-expired",
		RuntimeVersion: version,
		Status:         domain.PodStatusCreating,
		ExpiresAt:      fixedNow().Add(-time.Second),
	})
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "viewer-ps-expired",
			CreationTimestamp: metav1.NewTime(fixedNow().Add(-time.Minute)),
			Labels: map[string]string{
				labelComponent:      componentViewer,
				labelPVCName:        "data",
				labelPVCUID:         "uid",
				labelPodSessionID:   "ps_expired",
				labelRuntimeVersion: version,
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
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
	synced, ok := store.GetPodSessionIncludingExpired("ps_expired")
	if !ok {
		t.Fatal("expired stored pod session was purged during reconcile")
	}
	if synced.Status != domain.PodStatusReady {
		t.Fatalf("reconciled pod session status = %q, want %q", synced.Status, domain.PodStatusReady)
	}
}

func TestReconcileViewerPodsSkipsRecentPodWithDifferentRuntimeVersion(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "viewer-ps-old",
			CreationTimestamp: metav1.NewTime(fixedNow().Add(-time.Minute)),
			Labels: map[string]string{
				labelComponent:      componentViewer,
				labelPVCName:        "data",
				labelPVCUID:         "uid",
				labelPodSessionID:   "ps_old",
				labelRuntimeVersion: "old-version",
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
	if _, ok := store.GetPodSession("ps_old", fixedNow()); ok {
		t.Fatal("recent pod with different runtime version was recovered into state")
	}
}

func TestReconcileViewerPodsDeletesOldPodWithDifferentRuntimeVersion(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "viewer-ps-old",
			CreationTimestamp: metav1.NewTime(fixedNow().Add(-cfg.Sessions.OrphanGrace - time.Second)),
			Labels: map[string]string{
				labelComponent:      componentViewer,
				labelPVCName:        "data",
				labelPVCUID:         "uid",
				labelPodSessionID:   "ps_old",
				labelRuntimeVersion: "old-version",
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	clientset := fake.NewSimpleClientset(pod)
	service := NewPodService(
		cfg,
		store,
		kube.New(clientset),
		observability.MustNew(cfg.Observability, nil),
	)
	service.now = fixedNow

	if err := service.ReconcileViewerPods(t.Context(), "default"); err != nil {
		t.Fatalf("ReconcileViewerPods() error = %v", err)
	}
	if _, err := clientset.CoreV1().Pods("default").Get(t.Context(), "viewer-ps-old", metav1.GetOptions{}); err == nil {
		t.Fatal("old pod with different runtime version was not deleted")
	}
}

func TestReconcileViewerPodsDeletesOldPodWithDifferentStoredRuntimeVersion(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	store.PutPodSession(&domain.PodSession{
		ID:             "ps_old",
		Namespace:      "default",
		PVCName:        "data",
		PVCUID:         "uid",
		PodName:        "viewer-ps-old",
		ServiceName:    "viewer-ps-old",
		RuntimeVersion: "old-version",
		Status:         domain.PodStatusReady,
		ExpiresAt:      fixedNow().Add(time.Minute),
	})
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "viewer-ps-old",
			CreationTimestamp: metav1.NewTime(fixedNow().Add(-cfg.Sessions.OrphanGrace - time.Second)),
			Labels: map[string]string{
				labelComponent:      componentViewer,
				labelPVCName:        "data",
				labelPVCUID:         "uid",
				labelPodSessionID:   "ps_old",
				labelRuntimeVersion: "old-version",
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	clientset := fake.NewSimpleClientset(pod)
	service := NewPodService(
		cfg,
		store,
		kube.New(clientset),
		observability.MustNew(cfg.Observability, nil),
	)
	service.now = fixedNow

	if err := service.ReconcileViewerPods(t.Context(), "default"); err != nil {
		t.Fatalf("ReconcileViewerPods() error = %v", err)
	}
	if _, err := clientset.CoreV1().Pods("default").Get(t.Context(), "viewer-ps-old", metav1.GetOptions{}); err == nil {
		t.Fatal("old pod with different stored runtime version was not deleted")
	}
}

func viewerPodFixture(namespace string, name string, podSessionID string) *corev1.Pod {
	cfg := testConfig()
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				labelComponent:      componentViewer,
				labelPVCName:        "data",
				labelPVCUID:         "uid",
				labelPodSessionID:   podSessionID,
				labelRuntimeVersion: runtimeVersion(cfg),
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
