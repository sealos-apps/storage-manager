package session

import (
	"context"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

type staticLogin struct {
	token string
}

func (s staticLogin) Login(_ context.Context, _ string, _ string, _ string) (string, error) {
	return s.token, nil
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

func TestHeartbeatExtendsSession(t *testing.T) {
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
	service.now = fixedNow
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_1",
		PodSessionID: "ps_1",
		ExpiresAt:    fixedNow().Add(cfg.Sessions.ViewerSessionTimout),
	})

	heartbeat, err := service.HeartbeatForUser("vs_1", "")
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if !heartbeat.ExpiresAt.After(fixedNow()) {
		t.Fatalf("heartbeat = %#v", heartbeat)
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
	auth := NewAuthService(cfg, store, staticLogin{token: "fb-token"}, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, auth, observability.MustNew(cfg.Observability, nil))
	now := fixedNow()
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
	if _, err := service.HeartbeatForUser("vs_1", "other"); err == nil {
		t.Fatal("HeartbeatForUser() allowed another user")
	}
	if _, err := service.CloseViewerSessionForUser("vs_1", "other"); err == nil {
		t.Fatal("CloseViewerSessionForUser() allowed another user")
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
