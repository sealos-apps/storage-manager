package session

import (
	"strings"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsurePodSessionCreatesResources(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	clientset := fake.NewSimpleClientset()
	client := kube.New(clientset)
	service := NewPodService(cfg, store, client, observability.New(cfg.Observability, nil))
	service.now = fixedNow

	podSession, err := service.EnsurePodSession(t.Context(), EnsurePodSessionInput{
		Namespace:  "default",
		PVCName:    "data",
		PVCUID:     "uid",
		AccessMode: domain.AccessModeReadWriteOnce,
		Mode:       domain.ModeReadWrite,
		MountInfo:  &domain.PVCMountInfo{Mounted: true, Nodes: []string{"node-a"}},
	})
	if err != nil {
		t.Fatalf("EnsurePodSession() error = %v", err)
	}
	if podSession.ID == "" || podSession.Status != domain.PodStatusCreating {
		t.Fatalf("pod session = %#v", podSession)
	}
	if strings.Contains(podSession.PodName, "_") || strings.Contains(podSession.ViewerURL, "_") {
		t.Fatalf("kubernetes resource identifiers must be DNS-safe: pod=%q url=%q", podSession.PodName, podSession.ViewerURL)
	}

	pod, err := client.GetPod(t.Context(), "default", podSession.PodName)
	if err != nil {
		t.Fatalf("GetPod() error = %v", err)
	}
	if pod.Spec.Affinity == nil {
		t.Fatal("expected node affinity for RWO mounted PVC")
	}
	if pod.Spec.Containers[0].Image != cfg.Viewer.FileBrowser.Image {
		t.Fatalf("image = %q", pod.Spec.Containers[0].Image)
	}
	if !strings.Contains(pod.Spec.Containers[0].Args[0], "--auth.command=/hooks/filebrowser-auth-hook.sh") {
		t.Fatalf("filebrowser command did not configure hook auth: %q", pod.Spec.Containers[0].Args[0])
	}
	if !strings.Contains(pod.Spec.Containers[0].Args[0], "'/filebrowser' config init") {
		t.Fatalf("filebrowser command did not use configured binary path: %q", pod.Spec.Containers[0].Args[0])
	}
	if pod.Spec.Volumes[0].PersistentVolumeClaim.ReadOnly {
		t.Fatal("readwrite mode mounted readonly")
	}
	if pod.Spec.Volumes[1].ConfigMap == nil || pod.Spec.Volumes[1].ConfigMap.Name != podSession.PodName {
		t.Fatalf("hook configmap volume missing: %#v", pod.Spec.Volumes)
	}
	hookConfigMap, err := clientset.CoreV1().ConfigMaps("default").Get(
		t.Context(),
		podSession.PodName,
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("hook configmap was not created: %v", err)
	}
	assertOwnedByPod(t, hookConfigMap.OwnerReferences, pod)
	serviceResource, err := clientset.CoreV1().Services("default").Get(
		t.Context(),
		podSession.ServiceName,
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("service was not created: %v", err)
	}
	assertOwnedByPod(t, serviceResource.OwnerReferences, pod)
	ingress, err := clientset.NetworkingV1().Ingresses("default").Get(
		t.Context(),
		podSession.ServiceName,
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("ingress was not created: %v", err)
	}
	assertOwnedByPod(t, ingress.OwnerReferences, pod)
}

func TestEnsurePodSessionReusesExistingViewerPod(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	existing := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "viewer-ps_existing",
			CreationTimestamp: metav1.NewTime(fixedNow()),
			Labels: map[string]string{
				labelComponent:    componentViewer,
				labelPVCUID:       "uid",
				labelPodSessionID: "ps_existing",
			},
		},
		Spec: corev1.PodSpec{NodeName: "node-a"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
	store := state.New(cfg.Cache)
	client := kube.New(fake.NewSimpleClientset(existing))
	service := NewPodService(cfg, store, client, observability.New(cfg.Observability, nil))
	service.now = fixedNow

	podSession, err := service.EnsurePodSession(t.Context(), EnsurePodSessionInput{
		Namespace:  "default",
		PVCName:    "data",
		PVCUID:     "uid",
		AccessMode: domain.AccessModeReadWriteMany,
		Mode:       domain.ModeReadWrite,
	})
	if err != nil {
		t.Fatalf("EnsurePodSession() error = %v", err)
	}
	if podSession.ID != "ps_existing" || podSession.Status != domain.PodStatusReady {
		t.Fatalf("pod session = %#v", podSession)
	}
}

func TestEnsurePodSessionSkipsTerminatingViewerPod(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	deletionTime := metav1.NewTime(fixedNow())
	existing := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "viewer-ps-terminating",
			CreationTimestamp: metav1.NewTime(fixedNow().Add(-time.Minute)),
			DeletionTimestamp: &deletionTime,
			Labels: map[string]string{
				labelComponent:    componentViewer,
				labelPVCUID:       "uid",
				labelPodSessionID: "ps_terminating",
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	store := state.New(cfg.Cache)
	client := kube.New(fake.NewSimpleClientset(existing))
	service := NewPodService(cfg, store, client, observability.New(cfg.Observability, nil))
	service.now = fixedNow

	podSession, err := service.EnsurePodSession(t.Context(), EnsurePodSessionInput{
		Namespace:  "default",
		PVCName:    "data",
		PVCUID:     "uid",
		AccessMode: domain.AccessModeReadWriteMany,
		Mode:       domain.ModeReadWrite,
	})
	if err != nil {
		t.Fatalf("EnsurePodSession() error = %v", err)
	}
	if podSession.ID == "ps_terminating" {
		t.Fatal("terminating pod was reused")
	}
}

func TestBuildReadOnlyPod(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	service := NewPodService(
		cfg,
		state.New(cfg.Cache),
		kube.New(fake.NewSimpleClientset()),
		observability.New(cfg.Observability, nil),
	)
	pod := service.buildPod(&domain.PodSession{
		ID:        "ps_1",
		Namespace: "default",
		PVCName:   "data",
		PVCUID:    "uid",
		Mode:      domain.ModeReadOnly,
	}, nil)
	if !pod.Spec.Volumes[0].PersistentVolumeClaim.ReadOnly {
		t.Fatal("read-only mode did not set volume readonly")
	}
	if !pod.Spec.Containers[0].VolumeMounts[0].ReadOnly {
		t.Fatal("read-only mode did not set mount readonly")
	}
}

func TestSyncPodStatusReportsCrashLoop(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "viewer-ps-crash",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "filebrowser",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					},
				},
			},
		},
	}
	service := NewPodService(
		cfg,
		store,
		kube.New(fake.NewSimpleClientset(pod)),
		observability.New(cfg.Observability, nil),
	)

	updated, err := service.SyncPodStatus(t.Context(), &domain.PodSession{
		ID:        "ps_crash",
		Namespace: "default",
		PodName:   "viewer-ps-crash",
		ExpiresAt: fixedNow().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("SyncPodStatus() error = %v", err)
	}
	if updated.Status != domain.PodStatusFailed || updated.Reason != "CrashLoopBackOff" {
		t.Fatalf("updated = %#v", updated)
	}
}

func TestClosePodSessionTreatsMissingPodAsClosed(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	service := NewPodService(
		cfg,
		store,
		kube.New(fake.NewSimpleClientset()),
		observability.New(cfg.Observability, nil),
	)
	service.now = fixedNow
	store.PutPodSession(&domain.PodSession{
		ID:          "ps_missing",
		Namespace:   "default",
		PodName:     "viewer-ps-missing",
		ServiceName: "viewer-ps-missing",
		ExpiresAt:   fixedNow().Add(time.Minute),
	})

	closed, err := service.ClosePodSession(t.Context(), "ps_missing")
	if err != nil {
		t.Fatalf("ClosePodSession() error = %v", err)
	}
	if closed.Status != domain.PodStatusTerminated {
		t.Fatalf("closed session = %#v", closed)
	}
	if _, ok := store.GetPodSessionIncludingExpired("ps_missing"); ok {
		t.Fatal("closed pod session remained in state")
	}
}

func TestHookConfigMapUsesConfiguredScript(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Viewer.HookScript = "#!/bin/sh\necho configured-hook\n"
	service := NewPodService(
		cfg,
		state.New(cfg.Cache),
		kube.New(fake.NewSimpleClientset()),
		observability.New(cfg.Observability, nil),
	)
	configMap := service.buildHookConfigMap(&domain.PodSession{
		ID:        "ps_1",
		Namespace: "default",
		PodName:   "viewer-ps-1",
		PVCName:   "data",
		PVCUID:    "uid",
	}, metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       "viewer-ps-1",
		UID:        types.UID("pod-uid"),
	})

	if got := configMap.Data["filebrowser-auth-hook.sh"]; got != cfg.Viewer.HookScript {
		t.Fatalf("hook script = %q", got)
	}
}

func TestDNSLabelSanitizesGeneratedSessionID(t *testing.T) {
	t.Parallel()

	if got := resourceName("viewer-ps_ABC123"); got != "viewer-ps-abc123" {
		t.Fatalf("resourceName() = %q", got)
	}
	service := NewPodService(
		testConfig(),
		state.New(testConfig().Cache),
		kube.New(fake.NewSimpleClientset()),
		observability.New(testConfig().Observability, nil),
	)
	host, err := service.viewerHost("ps_ABC123")
	if err != nil {
		t.Fatalf("viewerHost() error = %v", err)
	}
	if strings.Contains(host, "_") {
		t.Fatalf("viewerHost() contains underscore: %q", host)
	}
}

func TestPodOwnerReferenceIncludesPodIdentity(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "viewer-ps-1",
			UID:  types.UID("pod-uid"),
		},
	}

	owner := podOwnerReference(pod)
	if owner.APIVersion != "v1" || owner.Kind != "Pod" || owner.Name != pod.Name || owner.UID != pod.UID {
		t.Fatalf("owner reference = %#v", owner)
	}
}

func assertOwnedByPod(t *testing.T, refs []metav1.OwnerReference, pod *corev1.Pod) {
	t.Helper()

	if len(refs) != 1 {
		t.Fatalf("owner references = %#v", refs)
	}
	owner := refs[0]
	if owner.APIVersion != "v1" || owner.Kind != "Pod" || owner.Name != pod.Name || owner.UID != pod.UID {
		t.Fatalf("owner reference = %#v, pod = %#v", owner, pod.ObjectMeta)
	}
}

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Viewer.HookClientToken = "hook-token"
	cfg.Viewer.BackendVerifyURL = "http://backend/internal/filebrowser-hook/verify"
	cfg.Viewer.HookScript = "#!/bin/sh\necho hook.action=block\n"
	cfg.Viewer.FileBrowser.Image = "filebrowser/filebrowser:v2.30.0"
	cfg.Viewer.FileBrowser.BinaryPath = "/filebrowser"
	cfg.Viewer.FileBrowser.Port = 8080
	cfg.Viewer.FileBrowser.TokenTTL = 15 * time.Minute
	cfg.Viewer.FileBrowser.LoginTimeout = 2 * time.Second
	cfg.Viewer.Pod.MountPath = "/srv"
	cfg.Viewer.Pod.DatabasePath = "/tmp/filebrowser.db"
	cfg.Viewer.Pod.CPURequest = "50m"
	cfg.Viewer.Pod.MemoryRequest = "64Mi"
	cfg.Viewer.Pod.CPULimit = "500m"
	cfg.Viewer.Pod.MemoryLimit = "512Mi"
	cfg.Viewer.Service.Type = "ClusterIP"
	cfg.Viewer.Service.Port = 80
	cfg.Viewer.Ingress.ClassName = "nginx"
	cfg.Viewer.Ingress.HostTemplate = "viewer-{{ .PodSessionID }}.example.test"
	return cfg
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
}
