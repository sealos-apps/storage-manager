package viewer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-stroage-manager/internal/config"
	"github.com/nixieboluo/sealos-stroage-manager/internal/domain"
	"github.com/nixieboluo/sealos-stroage-manager/internal/observability"
	"github.com/nixieboluo/sealos-stroage-manager/internal/session"
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

func (f *fakeViewerService) GetViewerSession(ctx context.Context, id string) (*domain.ViewerSession, error) {
	return f.created, nil
}

func (f *fakeViewerService) IssueToken(ctx context.Context, id string, userID string) (*domain.ViewerToken, error) {
	return f.token, nil
}

func (f *fakeViewerService) Heartbeat(id string) (*domain.Heartbeat, error) {
	return f.heartbeat, nil
}

func (f *fakeViewerService) CloseViewerSession(id string) (*domain.ViewerSession, error) {
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

func TestHandlerListPVCsUsesEnvelope(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			pvcs: []domain.PVC{{Namespace: "ns", Name: "data", MountedPods: []domain.MountedPod{}}},
		},
		fakePodService{},
		fakeAuthService{},
		observability.New(testObservability(), nil),
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
			token: &domain.ViewerToken{
				ViewerSessionID: "vs_1",
				Token:           "fb-token",
				TokenType:       "Bearer",
				ExpiresAt:       time.Now().Add(time.Minute),
			},
		},
		fakePodService{},
		fakeAuthService{},
		observability.New(testObservability(), nil),
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
		observability.New(testObservability(), nil),
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

func testObservability() config.ObservabilityConfig {
	return config.ObservabilityConfig{LogLevel: "error"}
}
