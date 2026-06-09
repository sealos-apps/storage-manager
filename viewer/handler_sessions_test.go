package viewer

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"encore.dev/beta/errs"
	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandlerIssueTokenNoStore(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			created: &domain.ViewerSession{
				ID:           "vs_1",
				PodSessionID: "ps_1",
				Namespace:    "ns",
				PVCName:      "data",
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
	req := httptest.NewRequest(http.MethodPost, "/viewer-sessions/vs_1/token", nil)
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

func TestHandlerFileManagementDisabledBlocksSessionEndpoints(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			created: &domain.ViewerSession{
				ID:           "vs_1",
				PodSessionID: "ps_1",
				Namespace:    "ns",
				PVCName:      "data",
			},
			heartbeat: &domain.Heartbeat{ViewerSessionID: "vs_1"},
			podSession: &domain.PodSession{
				ID:        "ps_1",
				Namespace: "ns",
				PVCName:   "data",
			},
			token: &domain.ViewerToken{ViewerSessionID: "vs_1"},
		},
		fakePodService{
			closed: &domain.PodSession{ID: "ps_1"},
		},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithFeatureConfig(testDisabledFileManagement()),
	)
	tests := []struct {
		name   string
		req    *http.Request
		handle func(*Handler, http.ResponseWriter, *http.Request)
	}{
		{
			name:   "create viewer session",
			req:    httptest.NewRequest(http.MethodPost, "/viewer-sessions", strings.NewReader(`{"namespace":"ns","pvc_name":"data"}`)),
			handle: (*Handler).CreateViewerSession,
		},
		{
			name:   "get viewer session",
			req:    httptest.NewRequest(http.MethodGet, "/viewer-sessions/vs_1", nil),
			handle: (*Handler).GetViewerSession,
		},
		{
			name:   "issue token",
			req:    httptest.NewRequest(http.MethodPost, "/viewer-sessions/vs_1/token", nil),
			handle: (*Handler).IssueToken,
		},
		{
			name:   "heartbeat",
			req:    httptest.NewRequest(http.MethodPost, "/viewer-sessions/vs_1/heartbeat", nil),
			handle: (*Handler).Heartbeat,
		},
		{
			name:   "close viewer",
			req:    httptest.NewRequest(http.MethodDelete, "/viewer-sessions/vs_1", nil),
			handle: (*Handler).CloseViewerSession,
		},
		{
			name:   "get pod",
			req:    httptest.NewRequest(http.MethodGet, "/pod-sessions/ps_1", nil),
			handle: (*Handler).GetPodSession,
		},
		{
			name:   "close pod",
			req:    httptest.NewRequest(http.MethodDelete, "/pod-sessions/ps_1", nil),
			handle: (*Handler).ClosePodSession,
		},
		{
			name: "verify hook",
			req: httptest.NewRequest(
				http.MethodPost,
				"/internal/filebrowser-hook/verify",
				strings.NewReader(`{"pod_session_id":"ps_1","viewer_pod_name":"viewer","username":"vs_1","auth_request_id":"ar_1","password_hash":"hash"}`),
			),
			handle: (*Handler).VerifyFileBrowserHook,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
			recorder := httptest.NewRecorder()

			tt.handle(handler, recorder, tt.req)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), string(apienv.CodeFileManagementDisabled)) {
				t.Fatalf("body = %s", recorder.Body.String())
			}
		})
	}
}

func TestHandlerIssueTokenAuthorizesFromViewerSessionPVC(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			created: &domain.ViewerSession{
				ID:           "vs_1",
				PodSessionID: "ps_missing",
				Namespace:    "ns",
				PVCName:      "data",
			},
			token: &domain.ViewerToken{
				ViewerSessionID: "vs_1",
				PodSessionID:    "ps_missing",
				Token:           "fb-token",
				TokenType:       "Bearer",
				ExpiresAt:       time.Now().Add(time.Minute),
			},
			podSessionErr: apienv.NewError(404, apienv.CodePodSessionNotFound, "Pod session no longer exists", nil),
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(http.MethodPost, "/viewer-sessions/vs_1/token", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.IssueToken(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerAdminSessionFollowUpAPIsUseExistingSessionEndpoints(t *testing.T) {
	t.Parallel()

	tokenCall := viewerSessionCall{}
	heartbeatCall := viewerSessionCall{}
	closeCall := viewerSessionCall{}
	principal, err := authn.PrincipalFromAuthorization(url.QueryEscape(testKubeconfig))
	if err != nil {
		t.Fatalf("PrincipalFromAuthorization() error = %v", err)
	}
	handler := NewHandler(
		&fakeViewerService{
			namespaces: []corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
			},
			created: &domain.ViewerSession{
				ID:           "vs_1",
				PodSessionID: "ps_1",
				Namespace:    "kube-system",
				PVCName:      "data",
				AdminContext: true,
			},
			token: &domain.ViewerToken{
				ViewerSessionID: "vs_1",
				PodSessionID:    "ps_1",
				Token:           "fb-token",
				TokenType:       "Bearer",
				ExpiresAt:       time.Now().Add(time.Minute),
			},
			tokenInput: &tokenCall,
			heartbeat: &domain.Heartbeat{
				ViewerSessionID: "vs_1",
				ExpiresAt:       time.Now().Add(time.Minute),
				Status:          domain.ViewerStatusReady,
			},
			heartbeatInput: &heartbeatCall,
			closed: &domain.ViewerSession{
				ID:           "vs_1",
				PodSessionID: "ps_1",
				Namespace:    "kube-system",
				PVCName:      "data",
				Status:       domain.ViewerStatusClosed,
				AdminContext: true,
			},
			closeInput: &closeCall,
			podSession: &domain.PodSession{
				ID:           "ps_1",
				Namespace:    "kube-system",
				PVCName:      "data",
				AdminContext: true,
			},
		},
		fakePodService{
			closed: &domain.PodSession{
				ID:           "ps_1",
				Namespace:    "kube-system",
				PVCName:      "data",
				Status:       domain.PodStatusTerminated,
				AdminContext: true,
			},
		},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
	)
	tests := []struct {
		name   string
		req    *http.Request
		handle func(*Handler, http.ResponseWriter, *http.Request)
	}{
		{
			name:   "token",
			req:    httptest.NewRequest(http.MethodPost, "/viewer-sessions/vs_1/token", nil),
			handle: (*Handler).IssueToken,
		},
		{
			name:   "heartbeat",
			req:    httptest.NewRequest(http.MethodPost, "/viewer-sessions/vs_1/heartbeat", nil),
			handle: (*Handler).Heartbeat,
		},
		{
			name:   "close viewer",
			req:    httptest.NewRequest(http.MethodDelete, "/viewer-sessions/vs_1", nil),
			handle: (*Handler).CloseViewerSession,
		},
		{
			name:   "close pod",
			req:    httptest.NewRequest(http.MethodDelete, "/pod-sessions/ps_1", nil),
			handle: (*Handler).ClosePodSession,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
			recorder := httptest.NewRecorder()

			tt.handle(handler, recorder, tt.req)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if tokenCall != (viewerSessionCall{id: "vs_1", userID: principal.ID}) {
		t.Fatalf("token call = %#v", tokenCall)
	}
	if heartbeatCall != (viewerSessionCall{id: "vs_1", userID: principal.ID}) {
		t.Fatalf("heartbeat call = %#v", heartbeatCall)
	}
	if closeCall != (viewerSessionCall{id: "vs_1", userID: principal.ID}) {
		t.Fatalf("close call = %#v", closeCall)
	}
}

func TestHandlerAdminSessionFollowUpDeniedWhenAdminAccessRevoked(t *testing.T) {
	t.Parallel()

	var tokenCall viewerSessionCall
	handler := NewHandler(
		&fakeViewerService{
			namespaces: []corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
			},
			created: &domain.ViewerSession{
				ID:           "vs_1",
				PodSessionID: "ps_1",
				Namespace:    "kube-system",
				PVCName:      "data",
				AdminContext: true,
			},
			tokenInput: &tokenCall,
			token:      &domain.ViewerToken{ViewerSessionID: "vs_1", Token: "fb-token"},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(denyTestAdminAuthorizer{}),
	)
	req := httptest.NewRequest(http.MethodPost, "/viewer-sessions/vs_1/token", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.IssueToken(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), string(apienv.CodeAdminAccessDenied)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if tokenCall != (viewerSessionCall{}) {
		t.Fatalf("token issued after admin denial: %#v", tokenCall)
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
	if details.Message != "File Browser login failed" {
		t.Fatalf("message = %q", details.Message)
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
