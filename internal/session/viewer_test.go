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
	storagev1 "k8s.io/api/storage/v1"
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

func TestViewerServiceCreatePVC(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(&storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standard",
			Annotations: map[string]string{
				StorageClassVisibleAnnotation:     "true",
				StorageClassAccessModesAnnotation: "ReadWriteOnce",
			},
		},
		Provisioner: "example.test/provisioner",
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	pvc, err := service.CreatePVC(t.Context(), CreatePVCInput{
		Namespace:        "default",
		Name:             "data",
		Capacity:         "2Gi",
		AccessModes:      []string{domain.AccessModeReadWriteOnce},
		StorageClassName: "standard",
	})
	if err != nil {
		t.Fatalf("CreatePVC() error = %v", err)
	}
	if pvc.Name != "data" || pvc.Capacity != "2Gi" {
		t.Fatalf("pvc = %#v", pvc)
	}
	created, err := clientset.CoreV1().PersistentVolumeClaims("default").Get(t.Context(), "data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() created pvc error = %v", err)
	}
	if created.Spec.StorageClassName == nil || *created.Spec.StorageClassName != "standard" {
		t.Fatalf("storage class = %#v", created.Spec.StorageClassName)
	}
}

func TestViewerServiceCreatePVCRejectsHiddenStorageClass(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := kube.New(fake.NewSimpleClientset(&storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "hidden"},
		Provisioner: "example.test/provisioner",
	}))
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	_, err := service.CreatePVC(t.Context(), CreatePVCInput{
		Namespace:        "default",
		Name:             "data",
		Capacity:         "2Gi",
		AccessModes:      []string{domain.AccessModeReadWriteOnce},
		StorageClassName: "hidden",
	})

	var apiErr *apienv.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("CreatePVC() error = %T %v, want apienv.Error", err, err)
	}
	if apiErr.Code != apienv.CodeStorageClassNotVisible {
		t.Fatalf("code = %s, want %s", apiErr.Code, apienv.CodeStorageClassNotVisible)
	}
}

func TestViewerServiceCreatePVCRejectsAccessModeOutsideStorageClassPolicy(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := kube.New(fake.NewSimpleClientset(&storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standard",
			Annotations: map[string]string{
				StorageClassVisibleAnnotation:     "true",
				StorageClassAccessModesAnnotation: "ReadWriteOnce",
			},
		},
		Provisioner: "example.test/provisioner",
	}))
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	_, err := service.CreatePVC(t.Context(), CreatePVCInput{
		Namespace:        "default",
		Name:             "data",
		Capacity:         "2Gi",
		AccessModes:      []string{domain.AccessModeReadWriteMany},
		StorageClassName: "standard",
	})

	var apiErr *apienv.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("CreatePVC() error = %T %v, want apienv.Error", err, err)
	}
	if apiErr.Code != apienv.CodeUnsupportedAccessMode {
		t.Fatalf("code = %s, want %s", apiErr.Code, apienv.CodeUnsupportedAccessMode)
	}
}

func TestViewerServiceDeletePVCRejectsMountedPVC(t *testing.T) {
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

	if _, err := service.DeletePVC(t.Context(), DeletePVCInput{Namespace: "default", Name: "data"}); err == nil {
		t.Fatal("DeletePVC() error = nil")
	}
}

func TestViewerServiceExpandPVC(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	pvc, err := service.ExpandPVC(t.Context(), ExpandPVCInput{
		Namespace: "default",
		Name:      "data",
		Capacity:  "3Gi",
	})
	if err != nil {
		t.Fatalf("ExpandPVC() error = %v", err)
	}
	if pvc.Capacity != "3Gi" {
		t.Fatalf("pvc = %#v", pvc)
	}
	updated, err := clientset.CoreV1().PersistentVolumeClaims("default").Get(t.Context(), "data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() updated pvc error = %v", err)
	}
	if updated.Spec.Resources.Requests.Storage().String() != "3Gi" {
		t.Fatalf("storage = %s", updated.Spec.Resources.Requests.Storage().String())
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
