package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/filebrowser"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

type staticLogin struct {
	token string
}

func (s staticLogin) Login(_ context.Context, _ string, _ string, _ string) (string, error) {
	return s.token, nil
}

type hookVerifyingLogin struct {
	auth *AuthService
}

func (l hookVerifyingLogin) Login(_ context.Context, _ string, username string, password string) (string, error) {
	parts := strings.SplitN(password, ".", 2)
	if len(parts) != 2 {
		return "", errors.New("password missing auth request secret")
	}
	result := l.auth.VerifyHook(HookVerifyInput{
		HookClientToken: testConfig().Viewer.HookClientToken,
		PodSessionID:    "ps_1",
		ViewerPodName:   "viewer-ps-1",
		Username:        username,
		AuthRequestID:   parts[0],
		PasswordHash:    filebrowser.HashSecret(parts[1]),
	})
	if !result.Allow {
		return "", errors.New(result.Reason)
	}
	return "fb-token", nil
}

func TestViewerServiceListPVCs(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := kube.New(fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
		testMountedPod("default", "app", "node-a", "data"),
	))
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	items, err := service.ListPVCs(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListPVCs() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	if !items[0].ViewerScheduling.RequiresNode || items[0].ViewerScheduling.NodeName != "node-a" {
		t.Fatalf("scheduling = %#v", items[0].ViewerScheduling)
	}
}

func TestViewerServiceListPVCsDetectsMountsInOnePodList(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data-a", UID: types.UID("uid-a")},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data-b", UID: types.UID("uid-b")},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
		testMountedPod("default", "app-a", "node-a", "data-a"),
		testMountedPod("default", "app-b", "node-b", "data-b"),
	)
	podListCalls := 0
	clientset.PrependReactor("list", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		podListCalls++
		return false, nil, nil
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	items, err := service.ListPVCs(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListPVCs() error = %v", err)
	}
	if podListCalls != 1 {
		t.Fatalf("pod list calls = %d, want 1", podListCalls)
	}
	if len(items) != 2 || !items[0].Mounted || !items[1].Mounted {
		t.Fatalf("items = %#v", items)
	}
}

func TestViewerServiceListPVCsSkipsMountDetectionWhenDisabled(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Viewer.PVCMountDetection.Enabled = false
	clientset := fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
		testMountedPod("default", "app", "node-a", "data"),
	)
	podListCalls := 0
	clientset.PrependReactor("list", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		podListCalls++
		return false, nil, nil
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	items, err := service.ListPVCs(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListPVCs() error = %v", err)
	}
	if podListCalls != 0 {
		t.Fatalf("pod list calls = %d, want 0", podListCalls)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	if items[0].Mounted || items[0].MountStatus != domain.PVCMountStatusUnknown || len(items[0].MountedPods) != 0 {
		t.Fatalf("item = %#v", items[0])
	}
}

func TestViewerServiceListPVCsAddsReadyPVCVolumeStats(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := kube.New(fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			},
		},
	}))
	store := state.New(cfg.Cache)
	recorder := observability.MustNew(cfg.Observability, nil)
	pods := NewPodService(cfg, store, client, recorder)
	service := NewViewerService(cfg, store, client, pods, nil, recorder, WithPVCMetrics(fakePVCMetrics{
		stats: map[string]domain.PVCVolumeStats{
			"data": {
				Source:              "kubelet",
				Status:              "ready",
				UsedBytes:           4 * 1024 * 1024 * 1024,
				MetricCapacityBytes: 10 * 1024 * 1024 * 1024,
				AvailableBytes:      6 * 1024 * 1024 * 1024,
			},
		},
	}))

	items, err := service.ListPVCs(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListPVCs() error = %v", err)
	}
	if items[0].VolumeStats == nil {
		t.Fatal("volume stats = nil")
	}
	if items[0].VolumeStats.Status != "ready" || items[0].VolumeStats.UsedBytes != 4*1024*1024*1024 {
		t.Fatalf("volume stats = %#v", items[0].VolumeStats)
	}
}

func TestViewerServiceListPVCsMarksMismatchedPVCVolumeStats(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := kube.New(fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}))
	store := state.New(cfg.Cache)
	recorder := observability.MustNew(cfg.Observability, nil)
	pods := NewPodService(cfg, store, client, recorder)
	service := NewViewerService(cfg, store, client, pods, nil, recorder, WithPVCMetrics(fakePVCMetrics{
		stats: map[string]domain.PVCVolumeStats{
			"data": {
				Source:              "kubelet",
				Status:              "ready",
				UsedBytes:           300 * 1024 * 1024 * 1024,
				MetricCapacityBytes: 418 * 1024 * 1024 * 1024,
				AvailableBytes:      90 * 1024 * 1024 * 1024,
			},
		},
	}))

	items, err := service.ListPVCs(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListPVCs() error = %v", err)
	}
	if items[0].VolumeStats == nil || items[0].VolumeStats.Status != "mismatched" {
		t.Fatalf("volume stats = %#v", items[0].VolumeStats)
	}
}

func TestViewerServiceListPVCsKeepsPVCsWhenPVCMetricsFail(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := kube.New(fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}))
	store := state.New(cfg.Cache)
	recorder := observability.MustNew(cfg.Observability, nil)
	pods := NewPodService(cfg, store, client, recorder)
	service := NewViewerService(cfg, store, client, pods, nil, recorder, WithPVCMetrics(fakePVCMetrics{
		err: errors.New("vmselect unavailable"),
	}))

	items, err := service.ListPVCs(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListPVCs() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	if items[0].VolumeStats != nil {
		t.Fatalf("volume stats = %#v, want nil", items[0].VolumeStats)
	}
}

func TestCreateViewerSessionRejectsRWOP(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := kube.New(fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
		},
	}))
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	if _, err := service.CreateViewerSession(t.Context(), CreateViewerSessionInput{
		Namespace: "default",
		PVCName:   "data",
		UserID:    "user",
	}); err == nil {
		t.Fatal("CreateViewerSession() error = nil")
	}
}

func TestCreateViewerSessionPropagatesAdminContext(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := kube.New(fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "data", UID: types.UID("uid")},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}))
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	viewer, err := service.CreateViewerSession(t.Context(), CreateViewerSessionInput{
		AdminContext: true,
		Namespace:    "kube-system",
		PVCName:      "data",
		UserID:       "admin",
	})
	if err != nil {
		t.Fatalf("CreateViewerSession() error = %v", err)
	}
	if !viewer.AdminContext {
		t.Fatalf("viewer.AdminContext = false")
	}
	podSession, ok := store.GetPodSession(viewer.PodSessionID, time.Now())
	if !ok {
		t.Fatalf("pod session missing")
	}
	if !podSession.AdminContext {
		t.Fatalf("podSession.AdminContext = false")
	}
}

func TestCreateViewerSessionPreservesTransientPVCGetErrors(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("get", "persistentvolumeclaims", func(_ ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("client rate limiter Wait returned an error: context canceled")
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	_, err := service.CreateViewerSession(t.Context(), CreateViewerSessionInput{
		Namespace: "default",
		PVCName:   "data",
		UserID:    "user",
	})

	if err == nil {
		t.Fatal("CreateViewerSession() error = nil")
	}
	var apiErr *apienv.Error
	if errors.As(err, &apiErr) && apiErr.Code == apienv.CodePVCNotFound {
		t.Fatalf("transient get error was mapped to PVC_NOT_FOUND: %#v", apiErr)
	}
	if !strings.Contains(err.Error(), "client rate limiter") {
		t.Fatalf("error = %v, want original transient error", err)
	}
}

func TestHeartbeatExtendsSession(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	podSession := &domain.PodSession{
		ID:             "ps_1",
		Namespace:      "default",
		PodName:        "viewer-ps-1",
		ServiceName:    "viewer-ps-1",
		PVCUID:         "uid",
		RuntimeVersion: runtimeVersion(cfg),
		Status:         domain.PodStatusReady,
		LastActiveAt:   fixedNow().Add(-time.Minute),
		ExpiresAt:      fixedNow().Add(time.Minute),
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Name:        "viewer-ps-1",
			Labels:      managedLabels(podSession),
			Annotations: lifecycleAnnotations(podSession),
		},
	}
	clientset := fake.NewSimpleClientset(pod)
	client := kube.New(clientset)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	pods.now = fixedNow
	service := NewViewerService(
		cfg,
		store,
		client,
		pods,
		nil,
		observability.MustNew(cfg.Observability, nil),
	)
	service.now = fixedNow
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_1",
		PodSessionID: "ps_1",
		ExpiresAt:    fixedNow().Add(cfg.Sessions.ViewerSessionTimout),
	})
	store.PutPodSession(podSession)

	heartbeat, err := service.HeartbeatForUser(t.Context(), "vs_1", "")
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if !heartbeat.ExpiresAt.After(fixedNow()) {
		t.Fatalf("heartbeat = %#v", heartbeat)
	}
	updated, err := clientset.CoreV1().Pods("default").Get(t.Context(), "viewer-ps-1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() heartbeat pod error = %v", err)
	}
	if updated.Annotations[annotationLastActiveAt] != fixedNow().Format(time.RFC3339Nano) {
		t.Fatalf("last active annotation = %q", updated.Annotations[annotationLastActiveAt])
	}
	if updated.Annotations[annotationKeepaliveUntil] != fixedNow().Add(cfg.Sessions.PodKeepaliveGrace).Format(time.RFC3339Nano) {
		t.Fatalf("keepalive annotation = %q", updated.Annotations[annotationKeepaliveUntil])
	}
}

func TestIssueTokenSyncsPodStatusBeforeReadinessCheck(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "viewer-ps-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
	client := kube.New(fake.NewSimpleClientset(pod))
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	auth := NewAuthService(cfg, store, nil, observability.MustNew(cfg.Observability, nil))
	auth.login = hookVerifyingLogin{auth: auth}
	service := NewViewerService(cfg, store, client, pods, auth, observability.MustNew(cfg.Observability, nil))
	now := fixedNow()
	auth.now = func() time.Time { return now }
	service.now = func() time.Time { return now }
	store.PutPodSession(&domain.PodSession{
		ID:        "ps_1",
		Namespace: "default",
		PodName:   "viewer-ps-1",
		ViewerURL: "http://viewer.example.test",
		Status:    domain.PodStatusCreating,
		ExpiresAt: now.Add(time.Minute),
	})
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_1",
		UserID:       "owner",
		PodSessionID: "ps_1",
		ExpiresAt:    now.Add(time.Minute),
	})

	token, err := service.IssueToken(t.Context(), "vs_1", "owner")
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}
	if token.Token != "fb-token" {
		t.Fatalf("token = %#v", token)
	}
}

func TestIssueTokenReplacesMissingPodSessionAndDeletesOldPod(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	version := runtimeVersion(cfg)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "viewer-ps_recovered",
			CreationTimestamp: metav1.NewTime(fixedNow()),
			Labels: map[string]string{
				labelComponent:      componentViewer,
				labelPVCUID:         "uid",
				labelPodSessionID:   "ps_recovered",
				labelRuntimeVersion: version,
			},
			Annotations: map[string]string{
				annotationRuntimeVersion: version,
				annotationKeepaliveUntil: fixedNow().Add(time.Minute).Format(time.RFC3339Nano),
			},
		},
		Spec: corev1.PodSpec{NodeName: "node-a"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	client := kube.New(fake.NewSimpleClientset(pvc, pod))
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	pods.now = fixedNow
	auth := NewAuthService(cfg, store, staticLogin{token: "fb-token"}, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, auth, observability.MustNew(cfg.Observability, nil))
	service.now = fixedNow
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_1",
		UserID:       "owner",
		PodSessionID: "ps_recovered",
		Namespace:    "default",
		PVCName:      "data",
		ExpiresAt:    fixedNow().Add(time.Minute),
	})

	_, err := service.IssueToken(t.Context(), "vs_1", "owner")
	var apiErr *apienv.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("IssueToken() error = %T %v, want apienv.Error", err, err)
	}
	if apiErr.Code != apienv.CodeViewerPodCreating {
		t.Fatalf("code = %s, want %s", apiErr.Code, apienv.CodeViewerPodCreating)
	}
	if _, err := client.GetPod(t.Context(), "default", "viewer-ps_recovered"); !apierrors.IsNotFound(err) {
		t.Fatalf("old viewer pod error = %v, want not found", err)
	}
	updated, ok := store.GetViewerSession("vs_1", fixedNow())
	if !ok {
		t.Fatal("viewer session missing")
	}
	if updated.PodSessionID == "ps_recovered" {
		t.Fatalf("viewer session still references old pod session: %#v", updated)
	}
	if _, ok := store.GetPodSession(updated.PodSessionID, fixedNow()); !ok {
		t.Fatal("replacement pod session was not stored")
	}
}

func TestIssueTokenUsesReadyReplacementPodSession(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
	clientset := fake.NewSimpleClientset(pvc)
	clientset.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		create := action.(ktesting.CreateAction)
		pod := create.GetObject().(*corev1.Pod).DeepCopy()
		pod.Status.Phase = corev1.PodRunning
		pod.Status.Conditions = []corev1.PodCondition{{
			Type:   corev1.PodReady,
			Status: corev1.ConditionTrue,
		}}
		if err := clientset.Tracker().Create(corev1.SchemeGroupVersion.WithResource("pods"), pod, pod.Namespace); err != nil {
			return true, nil, err
		}
		return true, pod, nil
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	pods.now = fixedNow
	auth := NewAuthService(cfg, store, staticLogin{token: "fb-token"}, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, auth, observability.MustNew(cfg.Observability, nil))
	service.now = fixedNow
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_1",
		UserID:       "owner",
		PodSessionID: "ps_missing",
		Namespace:    "default",
		PVCName:      "data",
		ExpiresAt:    fixedNow().Add(time.Minute),
	})

	token, err := service.IssueToken(t.Context(), "vs_1", "owner")
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}
	if token.Token != "fb-token" || token.PodSessionID == "ps_missing" {
		t.Fatalf("token = %#v", token)
	}
}

func TestViewerServiceRejectsCrossUserSessionAccess(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	service := NewViewerService(
		cfg,
		store,
		kube.New(fake.NewSimpleClientset()),
		nil,
		nil,
		observability.MustNew(cfg.Observability, nil),
	)
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_1",
		UserID:       "owner",
		PodSessionID: "ps_1",
		ExpiresAt:    fixedNow().Add(cfg.Sessions.ViewerSessionTimout),
	})

	if _, err := service.GetViewerSession(t.Context(), "vs_1", "other"); err == nil {
		t.Fatal("GetViewerSession() allowed another user")
	}
	if _, err := service.HeartbeatForUser(t.Context(), "vs_1", "other"); err == nil {
		t.Fatal("HeartbeatForUser() allowed another user")
	}
	if _, err := service.CloseViewerSessionForUser("vs_1", "other"); err == nil {
		t.Fatal("CloseViewerSessionForUser() allowed another user")
	}
}

func TestViewerServiceReturnsPodSessionNotFoundCode(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	service := NewViewerService(
		cfg,
		state.New(cfg.Cache),
		kube.New(fake.NewSimpleClientset()),
		nil,
		nil,
		observability.MustNew(cfg.Observability, nil),
	)

	_, err := service.GetPodSession("ps_missing")

	var apiErr *apienv.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("GetPodSession() error = %T %v, want apienv.Error", err, err)
	}
	if apiErr.Code != apienv.CodePodSessionNotFound {
		t.Fatalf("code = %s, want %s", apiErr.Code, apienv.CodePodSessionNotFound)
	}
}

type fakePVCMetrics struct {
	err   error
	stats map[string]domain.PVCVolumeStats
}

func (f fakePVCMetrics) ListPVCVolumeStats(context.Context, string) (map[string]domain.PVCVolumeStats, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.stats, nil
}

func testMountedPod(namespace string, name string, node string, pvc string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: corev1.PodSpec{
			NodeName: node,
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc,
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}
