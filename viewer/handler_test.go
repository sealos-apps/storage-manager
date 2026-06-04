package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"encore.dev/beta/errs"
	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type fakeViewerService struct {
	pvcs           []domain.PVC
	pvc            *domain.PVC
	storageClasses []domain.StorageClass
	created        *domain.ViewerSession
	token          *domain.ViewerToken
	heartbeat      *domain.Heartbeat
	closed         *domain.ViewerSession
	podSession     *domain.PodSession
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

func (f *fakeViewerService) ListPVCs(_ context.Context, _ string) ([]domain.PVC, error) {
	return f.pvcs, nil
}

func (f *fakeViewerService) CreatePVC(_ context.Context, _ session.CreatePVCInput) (*domain.PVC, error) {
	return f.pvc, nil
}

func (f *fakeViewerService) DeletePVC(_ context.Context, _ session.DeletePVCInput) (*domain.PVC, error) {
	return f.pvc, nil
}

func (f *fakeViewerService) ExpandPVC(_ context.Context, _ session.ExpandPVCInput) (*domain.PVC, error) {
	return f.pvc, nil
}

func (f *fakeViewerService) ListStorageClasses(_ context.Context) ([]domain.StorageClass, error) {
	return f.storageClasses, nil
}

func (f *fakeViewerService) CreateViewerSession(
	_ context.Context,
	_ session.CreateViewerSessionInput,
) (*domain.ViewerSession, error) {
	return f.created, nil
}

func (f *fakeViewerService) GetViewerSession(
	_ context.Context,
	_ string,
	_ string,
) (*domain.ViewerSession, error) {
	return f.created, nil
}

func (f *fakeViewerService) IssueToken(_ context.Context, _ string, _ string) (*domain.ViewerToken, error) {
	return f.token, nil
}

func (f *fakeViewerService) HeartbeatForUser(_ context.Context, _ string, _ string) (*domain.Heartbeat, error) {
	return f.heartbeat, nil
}

func (f *fakeViewerService) CloseViewerSessionForUser(_ string, _ string) (*domain.ViewerSession, error) {
	return f.closed, nil
}

func (f *fakeViewerService) GetPodSession(_ string) (*domain.PodSession, error) {
	return f.podSession, nil
}

type fakePodService struct {
	closed *domain.PodSession
}

func (f fakePodService) ClosePodSession(_ context.Context, _ string) (*domain.PodSession, error) {
	return f.closed, nil
}

type fakeAuthService struct {
	result domain.FileBrowserHookVerification
}

func (f fakeAuthService) VerifyHook(_ session.HookVerifyInput) domain.FileBrowserHookVerification {
	return f.result
}

type allowAuthorizer struct{}

var clientsetFactoryMu sync.Mutex

func (allowAuthorizer) CanListPVCs(_ context.Context, _ *authn.Principal, _ string) error {
	return nil
}

func (allowAuthorizer) CanGetPVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return nil
}

func (allowAuthorizer) CanCreatePVC(_ context.Context, _ *authn.Principal, _ string) error {
	return nil
}

func (allowAuthorizer) CanDeletePVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return nil
}

func (allowAuthorizer) CanUpdatePVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return nil
}

func (allowAuthorizer) CanListStorageClasses(_ context.Context, _ *authn.Principal) error {
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
		observability.MustNew(testObservability(), nil),
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

func TestHandlerGetContextUsesKubeconfigNamespace(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/context", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.GetContext(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"namespace":"ns"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerRejectsExplicitDifferentNamespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request func() *http.Request
		handle  func(*Handler, http.ResponseWriter, *http.Request)
	}{
		{
			name: "list pvcs",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/pvcs?namespace=other", nil)
			},
			handle: (*Handler).ListPVCs,
		},
		{
			name: "create pvc",
			request: func() *http.Request {
				return httptest.NewRequest(
					http.MethodPost,
					"/api/pvcs",
					strings.NewReader(`{"namespace":"other","name":"data","capacity":"10Gi","access_modes":["ReadWriteOnce"]}`),
				)
			},
			handle: (*Handler).CreatePVC,
		},
		{
			name: "expand pvc",
			request: func() *http.Request {
				return httptest.NewRequest(
					http.MethodPost,
					"/api/pvcs/other/data/expand",
					strings.NewReader(`{"capacity":"20Gi"}`),
				)
			},
			handle: (*Handler).ExpandPVC,
		},
		{
			name: "delete pvc",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodDelete, "/api/pvcs/other/data", nil)
			},
			handle: (*Handler).DeletePVC,
		},
		{
			name: "create viewer session",
			request: func() *http.Request {
				return httptest.NewRequest(
					http.MethodPost,
					"/api/viewer-sessions",
					strings.NewReader(`{"namespace":"other","pvc_name":"data"}`),
				)
			},
			handle: (*Handler).CreateViewerSession,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := NewHandler(
				&fakeViewerService{},
				fakePodService{},
				fakeAuthService{},
				nil,
				observability.MustNew(testObservability(), nil),
				allowAuthorizer{},
			)
			req := tt.request()
			req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
			recorder := httptest.NewRecorder()

			tt.handle(handler, recorder, req)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestHandlerCreatePVCUsesEnvelope(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			pvc: &domain.PVC{Namespace: "ns", Name: "data", Capacity: "10Gi"},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/pvcs",
		strings.NewReader(`{"namespace":"ns","name":"data","capacity":"10Gi","access_modes":["ReadWriteOnce"]}`),
	)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.CreatePVC(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"pvc"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerExpandPVCUsesPathParams(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			pvc: &domain.PVC{Namespace: "ns", Name: "data", Capacity: "20Gi"},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/pvcs/ns/data/expand",
		strings.NewReader(`{"capacity":"20Gi"}`),
	)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ExpandPVC(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"capacity":"20Gi"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerListStorageClassesUsesEnvelope(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			storageClasses: []domain.StorageClass{{Name: "standard", Provisioner: "test"}},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/storage-classes", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ListStorageClasses(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"storage_class_list"`) {
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
		observability.MustNew(testObservability(), nil),
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
		observability.MustNew(testObservability(), nil),
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

func TestToEncoreErrorPreservesBusinessCodeAndRetryableStatus(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	err := toEncoreError(apienv.NewError(502, apienv.CodeFileBrowserLoginFailed, "File Browser login failed", nil))

	var encoreErr *errs.Error
	if !errors.As(err, &encoreErr) {
		t.Fatalf("toEncoreError() = %T %v, want *errs.Error", err, err)
	}
	if encoreErr.Code != errs.Unavailable {
		t.Fatalf("encore code = %s, want unavailable", encoreErr.Code)
	}
	details, ok := encoreErr.Details.(ErrorDetails)
	if !ok {
		t.Fatalf("details = %T, want ErrorDetails", encoreErr.Details)
	}
	if details.Code != apienv.CodeFileBrowserLoginFailed {
		t.Fatalf("business code = %s", details.Code)
	}
}

func TestHandlerMetricsReturnsPrometheusText(t *testing.T) {
	t.Setenv("ENCORERUNTIME_NOPANIC", "1")

	recorder := observability.MustNew(testObservability(), nil)
	recorder.ObserveViewerSession("created")
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
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("content type = %q", contentType)
	}
	if !strings.Contains(response.Body.String(), `viewer_session_events_total{Event="created"} 1`) {
		t.Fatalf("metrics = %s", response.Body.String())
	}
}

func TestHandlerListPVCsDataReturnsDocumentedResponse(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			pvcs: []domain.PVC{{Namespace: "ns", Name: "data", MountedPods: []domain.MountedPod{}}},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)

	response, err := handler.ListPVCsData(t.Context(), &ListPVCsRequest{
		Authorization: url.QueryEscape(testKubeconfig),
		Namespace:     "ns",
	})

	if err != nil {
		t.Fatalf("ListPVCsData() error = %v", err)
	}
	if len(response.PVCList.Items) != 1 {
		t.Fatalf("response = %+v", response)
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
	authorizer := newKubernetesAuthorizer(
		fake.NewSimpleClientset(namespaceWithUID("ns", "managed-uid")),
		observability.MustNew(testObservability(), nil),
	)
	newClientset := kubernetesClientsetForConfig
	kubernetesClientsetForConfig = func(_ *rest.Config) (kubernetes.Interface, error) {
		return userClient, nil
	}
	defer func() {
		kubernetesClientsetForConfig = newClientset
	}()

	if err := authorizer.CanListPVCs(t.Context(), principal, "ns"); err == nil {
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
	authorizer := newKubernetesAuthorizer(
		fake.NewSimpleClientset(pvcWithUID("ns", "data", "managed-uid")),
		observability.MustNew(testObservability(), nil),
	)
	newClientset := kubernetesClientsetForConfig
	kubernetesClientsetForConfig = func(_ *rest.Config) (kubernetes.Interface, error) {
		return userClient, nil
	}
	defer func() {
		kubernetesClientsetForConfig = newClientset
	}()

	if err := authorizer.CanGetPVC(t.Context(), principal, "ns", "data"); err == nil {
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
	cfg := config.Default().Observability
	cfg.Logs.Exporter = "discard"
	cfg.Logs.Level = "error"
	return cfg
}
