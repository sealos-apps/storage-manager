package viewer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-stroage-manager/internal/authn"
	"github.com/nixieboluo/sealos-stroage-manager/internal/config"
	"github.com/nixieboluo/sealos-stroage-manager/internal/domain"
	"github.com/nixieboluo/sealos-stroage-manager/internal/observability"
	"github.com/nixieboluo/sealos-stroage-manager/internal/session"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type fakeViewerService struct {
	pvcs       []domain.PVC
	created    *domain.ViewerSession
	token      *domain.ViewerToken
	heartbeat  *domain.Heartbeat
	closed     *domain.ViewerSession
	podSession *domain.PodSession
}

const testKubeconfig = `apiVersion: v1
kind: Config
current-context: dev
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
users:
- name: u
  user:
    token: test-token
contexts:
- name: dev
  context:
    cluster: c
    user: u
    namespace: ns
`

func (f *fakeViewerService) ListPVCs(ctx context.Context, namespace string) ([]domain.PVC, error) {
	return f.pvcs, nil
}

func (f *fakeViewerService) CreateViewerSession(
	ctx context.Context,
	input session.CreateViewerSessionInput,
) (*domain.ViewerSession, error) {
	return f.created, nil
}

func (f *fakeViewerService) GetViewerSession(
	ctx context.Context,
	id string,
	userID string,
) (*domain.ViewerSession, error) {
	return f.created, nil
}

func (f *fakeViewerService) IssueToken(ctx context.Context, id string, userID string) (*domain.ViewerToken, error) {
	return f.token, nil
}

func (f *fakeViewerService) HeartbeatForUser(id string, userID string) (*domain.Heartbeat, error) {
	return f.heartbeat, nil
}

func (f *fakeViewerService) CloseViewerSessionForUser(id string, userID string) (*domain.ViewerSession, error) {
	return f.closed, nil
}

func (f *fakeViewerService) GetPodSession(id string) (*domain.PodSession, error) {
	return f.podSession, nil
}

type fakePodService struct {
	closed *domain.PodSession
}

func (f fakePodService) ClosePodSession(ctx context.Context, id string) (*domain.PodSession, error) {
	return f.closed, nil
}

type fakeAuthService struct {
	result domain.FileBrowserHookVerification
}

func (f fakeAuthService) VerifyHook(input session.HookVerifyInput) domain.FileBrowserHookVerification {
	return f.result
}

type allowAuthorizer struct{}

var clientsetFactoryMu sync.Mutex

func (allowAuthorizer) CanListPVCs(ctx context.Context, principal *authn.Principal, namespace string) error {
	return nil
}

func (allowAuthorizer) CanGetPVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
	name string,
) error {
	return nil
}

func TestHandlerListPVCsUsesEnvelope(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			pvcs: []domain.PVC{{Namespace: "ns", Name: "data", MountedPods: []domain.MountedPod{}}},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.New(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/pvcs?namespace=ns", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ListPVCs(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]struct {
		Items []domain.PVC `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body["pvc_list"].Items) != 1 {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerIssueTokenNoStore(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			created: &domain.ViewerSession{
				ID:           "vs_1",
				PodSessionID: "ps_1",
			},
			token: &domain.ViewerToken{
				ViewerSessionID: "vs_1",
				Token:           "fb-token",
				TokenType:       "Bearer",
				ExpiresAt:       time.Now().Add(time.Minute),
			},
			podSession: &domain.PodSession{
				ID:        "ps_1",
				Namespace: "ns",
				PVCName:   "data",
			},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.New(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(http.MethodPost, "/api/viewer-sessions/vs_1/token", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.IssueToken(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control = %q", recorder.Header().Get("Cache-Control"))
	}
	if strings.Contains(recorder.Body.String(), "kubeconfig") {
		t.Fatalf("body leaked sensitive data: %s", recorder.Body.String())
	}
}

func TestHandlerVerifyHookReturnsAllowEnvelope(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{result: domain.FileBrowserHookVerification{Allow: true, Scope: "/"}},
		nil,
		observability.New(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/filebrowser-hook/verify",
		strings.NewReader(`{"pod_session_id":"ps_1","username":"vs_1","auth_request_id":"ar_1","password_hash":"hash"}`),
	)
	req.Header.Set("Authorization", "Bearer hook-token")
	recorder := httptest.NewRecorder()

	handler.VerifyFileBrowserHook(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "filebrowser_hook_verification") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerMetricsReturnsPrometheusText(t *testing.T) {
	t.Parallel()

	recorder := observability.New(testObservability(), nil)
	recorder.Metrics().ViewerCreated.Add(1)
	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		recorder,
		allowAuthorizer{},
	)
	response := httptest.NewRecorder()

	handler.Metrics(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "viewer_sessions_created_total 1") {
		t.Fatalf("metrics = %s", response.Body.String())
	}
}

func TestKubernetesAuthorizerRequiresSameNamespaceUID(t *testing.T) {
	clientsetFactoryMu.Lock()
	defer clientsetFactoryMu.Unlock()

	principal, err := authn.PrincipalFromAuthorization(url.QueryEscape(testKubeconfig))
	if err != nil {
		t.Fatalf("PrincipalFromAuthorization() error = %v", err)
	}
	userClient := fake.NewSimpleClientset(namespaceWithUID("ns", "user-uid"))
	authorizer := newKubernetesAuthorizer(fake.NewSimpleClientset(namespaceWithUID("ns", "managed-uid")))
	newClientset := kubernetesClientsetForConfig
	kubernetesClientsetForConfig = func(_ *rest.Config) (kubernetes.Interface, error) {
		return userClient, nil
	}
	defer func() {
		kubernetesClientsetForConfig = newClientset
	}()

	if err := authorizer.CanListPVCs(context.Background(), principal, "ns"); err == nil {
		t.Fatal("CanListPVCs() allowed namespace UID mismatch")
	}
}

func TestKubernetesAuthorizerRequiresSamePVCUID(t *testing.T) {
	clientsetFactoryMu.Lock()
	defer clientsetFactoryMu.Unlock()

	principal, err := authn.PrincipalFromAuthorization(url.QueryEscape(testKubeconfig))
	if err != nil {
		t.Fatalf("PrincipalFromAuthorization() error = %v", err)
	}
	userClient := fake.NewSimpleClientset(pvcWithUID("ns", "data", "user-uid"))
	authorizer := newKubernetesAuthorizer(fake.NewSimpleClientset(pvcWithUID("ns", "data", "managed-uid")))
	newClientset := kubernetesClientsetForConfig
	kubernetesClientsetForConfig = func(_ *rest.Config) (kubernetes.Interface, error) {
		return userClient, nil
	}
	defer func() {
		kubernetesClientsetForConfig = newClientset
	}()

	if err := authorizer.CanGetPVC(context.Background(), principal, "ns", "data"); err == nil {
		t.Fatal("CanGetPVC() allowed PVC UID mismatch")
	}
}

func namespaceWithUID(name string, uid string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(uid),
		},
	}
}

func pvcWithUID(namespace string, name string, uid string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			UID:       types.UID(uid),
		},
	}
}

func testObservability() config.ObservabilityConfig {
	return config.ObservabilityConfig{LogLevel: "error"}
}
