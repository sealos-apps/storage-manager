package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"encore.dev/beta/errs"
	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var kubernetesClientsetForConfig = func(c *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(c)
}

type viewerService interface {
	ListPVCs(ctx context.Context, namespace string) ([]domain.PVC, error)
	CreateViewerSession(ctx context.Context, input session.CreateViewerSessionInput) (*domain.ViewerSession, error)
	GetViewerSession(ctx context.Context, id string, userID string) (*domain.ViewerSession, error)
	IssueToken(ctx context.Context, id string, userID string) (*domain.ViewerToken, error)
	HeartbeatForUser(id string, userID string) (*domain.Heartbeat, error)
	CloseViewerSessionForUser(id string, userID string) (*domain.ViewerSession, error)
	GetPodSession(id string) (*domain.PodSession, error)
}

type podService interface {
	ClosePodSession(ctx context.Context, podSessionID string) (*domain.PodSession, error)
}

type authService interface {
	VerifyHook(input session.HookVerifyInput) domain.FileBrowserHookVerification
}

type authorizer interface {
	CanListPVCs(ctx context.Context, principal *authn.Principal, namespace string) error
	CanGetPVC(ctx context.Context, principal *authn.Principal, namespace string, name string) error
}

type Handler struct {
	viewers  viewerService
	pods     podService
	auth     authService
	recorder *observability.Recorder
	authz    authorizer
}

type AuthenticatedRequest struct {
	Authorization string `header:"Authorization" encore:"sensitive"`
}

type ListPVCsRequest struct {
	Authorization string `header:"Authorization" encore:"sensitive"`
	Namespace     string `query:"namespace"`
}

type CreateViewerSessionRequest struct {
	Authorization string `header:"Authorization" encore:"sensitive"`
	Namespace     string `json:"namespace"`
	PVCName       string `json:"pvc_name"`
}

type VerifyFileBrowserHookRequest struct {
	Authorization string `header:"Authorization" encore:"sensitive"`
	PodSessionID  string `json:"pod_session_id"`
	ViewerPodName string `json:"viewer_pod_name"`
	Username      string `json:"username"`
	AuthRequestID string `json:"auth_request_id"`
	PasswordHash  string `json:"password_hash"`
}

type PVCList struct {
	Items []domain.PVC `json:"items"`
}

type ListPVCsResponse struct {
	PVCList PVCList `json:"pvc_list"`
}

type ViewerSessionResponse struct {
	ViewerSession *domain.ViewerSession `json:"viewer_session"`
}

type ViewerTokenResponse struct {
	CacheControl string              `header:"Cache-Control"`
	Pragma       string              `header:"Pragma"`
	ViewerToken  *domain.ViewerToken `json:"viewer_token"`
}

type HeartbeatResponse struct {
	Heartbeat *domain.Heartbeat `json:"heartbeat"`
}

type PodSessionResponse struct {
	PodSession *domain.PodSession `json:"pod_session"`
}

type FileBrowserHookVerificationResponse struct {
	FileBrowserHookVerification domain.FileBrowserHookVerification `json:"filebrowser_hook_verification"`
}

type MetricsSnapshot struct {
	HTTPRequests       int64 `json:"http_requests_total"`
	HTTPErrors         int64 `json:"http_errors_total"`
	ViewerCreated      int64 `json:"viewer_sessions_created_total"`
	ViewerClosed       int64 `json:"viewer_sessions_closed_total"`
	PodCreated         int64 `json:"pod_sessions_created_total"`
	PodReused          int64 `json:"pod_sessions_reused_total"`
	PodDeleted         int64 `json:"pod_sessions_deleted_total"`
	AuthCreated        int64 `json:"auth_requests_created_total"`
	AuthConsumed       int64 `json:"auth_requests_consumed_total"`
	AuthDenied         int64 `json:"auth_requests_denied_total"`
	FileBrowserLogins  int64 `json:"filebrowser_logins_total"`
	FileBrowserErrors  int64 `json:"filebrowser_errors_total"`
	KubernetesRequests int64 `json:"kubernetes_requests_total"`
	KubernetesErrors   int64 `json:"kubernetes_errors_total"`
	CleanupDeleted     int64 `json:"cleanup_deleted_total"`
}

type MetricsResponse struct {
	Metrics MetricsSnapshot `json:"metrics"`
}

type ErrorDetails struct {
	Code string `json:"code"`
}

func (ErrorDetails) ErrDetails() {}

func NewHandler(
	viewers viewerService,
	pods podService,
	auth authService,
	managementClient kubernetes.Interface,
	recorder *observability.Recorder,
	authz authorizer,
) *Handler {
	if authz == nil {
		authz = newKubernetesAuthorizer(managementClient)
	}
	return &Handler{
		viewers:  viewers,
		pods:     pods,
		auth:     auth,
		recorder: recorder,
		authz:    authz,
	}
}

func (h *Handler) ListPVCsData(ctx context.Context, req *ListPVCsRequest) (*ListPVCsResponse, error) {
	response, apiErr := h.listPVCs(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) CreateViewerSessionData(
	ctx context.Context,
	req *CreateViewerSessionRequest,
) (*ViewerSessionResponse, error) {
	response, apiErr := h.createViewerSession(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) GetViewerSessionData(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, error) {
	response, apiErr := h.getViewerSession(ctx, viewerSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) IssueTokenData(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerTokenResponse, error) {
	response, apiErr := h.issueToken(ctx, viewerSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) HeartbeatData(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*HeartbeatResponse, error) {
	response, apiErr := h.heartbeat(ctx, viewerSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) CloseViewerSessionData(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, error) {
	response, apiErr := h.closeViewerSession(ctx, viewerSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) ClosePodSessionData(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, error) {
	response, apiErr := h.closePodSession(ctx, podSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) GetPodSessionData(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, error) {
	response, apiErr := h.getPodSession(ctx, podSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) VerifyFileBrowserHookData(
	ctx context.Context,
	req *VerifyFileBrowserHookRequest,
) (*FileBrowserHookVerificationResponse, error) {
	response, apiErr := h.verifyFileBrowserHook(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) MetricsData(_ context.Context) (*MetricsResponse, error) {
	return &MetricsResponse{Metrics: metricsSnapshot(h.recorder)}, nil
}

func (h *Handler) ListPVCs(w http.ResponseWriter, req *http.Request) {
	response, err := h.listPVCs(req.Context(), &ListPVCsRequest{
		Authorization: req.Header.Get("Authorization"),
		Namespace:     req.URL.Query().Get("namespace"),
	})
	writeHTTPResponse(w, response, err)
}

func (h *Handler) CreateViewerSession(w http.ResponseWriter, req *http.Request) {
	var body CreateViewerSessionRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeHTTPResponse(w, nil, apienv.NewError(400, apienv.CodeValidationError, "Invalid JSON request", nil))
		return
	}
	body.Authorization = req.Header.Get("Authorization")
	response, err := h.createViewerSession(req.Context(), &body)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) GetViewerSession(w http.ResponseWriter, req *http.Request) {
	response, err := h.getViewerSession(
		req.Context(),
		pathID(req.URL.Path, "/api/viewer-sessions/"),
		authenticatedRequest(req),
	)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) IssueToken(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSuffix(pathID(req.URL.Path, "/api/viewer-sessions/"), "/token")
	response, err := h.issueToken(req.Context(), id, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) Heartbeat(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSuffix(pathID(req.URL.Path, "/api/viewer-sessions/"), "/heartbeat")
	response, err := h.heartbeat(req.Context(), id, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) CloseViewerSession(w http.ResponseWriter, req *http.Request) {
	response, err := h.closeViewerSession(
		req.Context(),
		pathID(req.URL.Path, "/api/viewer-sessions/"),
		authenticatedRequest(req),
	)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) ClosePodSession(w http.ResponseWriter, req *http.Request) {
	response, err := h.closePodSession(
		req.Context(),
		pathID(req.URL.Path, "/api/pod-sessions/"),
		authenticatedRequest(req),
	)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) GetPodSession(w http.ResponseWriter, req *http.Request) {
	response, err := h.getPodSession(
		req.Context(),
		pathID(req.URL.Path, "/api/pod-sessions/"),
		authenticatedRequest(req),
	)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) VerifyFileBrowserHook(w http.ResponseWriter, req *http.Request) {
	var body VerifyFileBrowserHookRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeHTTPResponse(w, nil, apienv.NewError(400, apienv.CodeValidationError, "Invalid JSON request", nil))
		return
	}
	body.Authorization = req.Header.Get("Authorization")
	response, err := h.verifyFileBrowserHook(req.Context(), &body)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) Metrics(w http.ResponseWriter, req *http.Request) {
	response, err := h.MetricsData(req.Context())
	writeHTTPResponse(w, response, apienv.FromError(err))
}

func (h *Handler) listPVCs(ctx context.Context, req *ListPVCsRequest) (*ListPVCsResponse, *apienv.Error) {
	start := time.Now()
	principal, err := authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/api/pvcs", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	namespace := req.Namespace
	if namespace == "" {
		namespace = principal.Namespace
	}
	if err := h.authz.CanListPVCs(ctx, principal, namespace); err != nil {
		apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
		h.observe(ctx, http.MethodGet, "/api/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	items, listErr := h.viewers.ListPVCs(ctx, namespace)
	if listErr != nil {
		apiErr := apienv.FromError(listErr)
		h.observe(ctx, http.MethodGet, "/api/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	if items == nil {
		items = []domain.PVC{}
	}
	h.observe(ctx, http.MethodGet, "/api/pvcs", http.StatusOK, start)
	return &ListPVCsResponse{PVCList: PVCList{Items: items}}, nil
}

func (h *Handler) createViewerSession(
	ctx context.Context,
	req *CreateViewerSessionRequest,
) (*ViewerSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	namespace := req.Namespace
	if namespace == "" {
		namespace = principal.Namespace
	}
	if req.PVCName == "" {
		apiErr := apienv.NewError(400, apienv.CodeValidationError, "pvc_name is required", nil)
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions", apiErr.Status, start)
		return nil, apiErr
	}
	if err := h.authz.CanGetPVC(ctx, principal, namespace, req.PVCName); err != nil {
		apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions", apiErr.Status, start)
		return nil, apiErr
	}
	viewerSession, createErr := h.viewers.CreateViewerSession(ctx, session.CreateViewerSessionInput{
		Namespace: namespace,
		PVCName:   req.PVCName,
		UserID:    principal.ID,
	})
	if createErr != nil {
		apiErr := apienv.FromError(createErr)
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/api/viewer-sessions", http.StatusCreated, start)
	return &ViewerSessionResponse{
		ViewerSession: viewerSession,
	}, nil
}

func (h *Handler) getViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/api/viewer-sessions/:id", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	viewerSession, getErr := h.viewers.GetViewerSession(ctx, viewerSessionID, principal.ID)
	if getErr != nil {
		apiErr := apienv.FromError(getErr)
		h.observe(ctx, http.MethodGet, "/api/viewer-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	if err := h.authorizePodSessionPVC(ctx, principal, viewerSession.PodSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodGet, "/api/viewer-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/api/viewer-sessions/:id", http.StatusOK, start)
	return &ViewerSessionResponse{ViewerSession: viewerSession}, nil
}

func (h *Handler) issueToken(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerTokenResponse, *apienv.Error) {
	start := time.Now()
	principal, err := authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions/:id/token", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authorizeViewerSessionPVC(ctx, principal, viewerSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions/:id/token", apiErr.Status, start)
		return nil, apiErr
	}
	token, issueErr := h.viewers.IssueToken(ctx, viewerSessionID, principal.ID)
	if issueErr != nil {
		apiErr := apienv.FromError(issueErr)
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions/:id/token", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/api/viewer-sessions/:id/token", http.StatusOK, start)
	return &ViewerTokenResponse{
		CacheControl: "no-store",
		Pragma:       "no-cache",
		ViewerToken:  token,
	}, nil
}

func (h *Handler) heartbeat(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*HeartbeatResponse, *apienv.Error) {
	start := time.Now()
	principal, err := authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions/:id/heartbeat", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authorizeViewerSessionPVC(ctx, principal, viewerSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions/:id/heartbeat", apiErr.Status, start)
		return nil, apiErr
	}
	heartbeat, heartbeatErr := h.viewers.HeartbeatForUser(viewerSessionID, principal.ID)
	if heartbeatErr != nil {
		apiErr := apienv.FromError(heartbeatErr)
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions/:id/heartbeat", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/api/viewer-sessions/:id/heartbeat", http.StatusOK, start)
	return &HeartbeatResponse{Heartbeat: heartbeat}, nil
}

func (h *Handler) closeViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodDelete, "/api/viewer-sessions/:id", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authorizeViewerSessionPVC(ctx, principal, viewerSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodDelete, "/api/viewer-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	viewerSession, closeErr := h.viewers.CloseViewerSessionForUser(viewerSessionID, principal.ID)
	if closeErr != nil {
		apiErr := apienv.FromError(closeErr)
		h.observe(ctx, http.MethodDelete, "/api/viewer-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodDelete, "/api/viewer-sessions/:id", http.StatusOK, start)
	return &ViewerSessionResponse{ViewerSession: viewerSession}, nil
}

func (h *Handler) closePodSession(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodDelete, "/api/pod-sessions/:id", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authorizePodSessionPVC(ctx, principal, podSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodDelete, "/api/pod-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	podSession, closeErr := h.pods.ClosePodSession(ctx, podSessionID)
	if closeErr != nil {
		apiErr := apienv.FromError(closeErr)
		h.observe(ctx, http.MethodDelete, "/api/pod-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodDelete, "/api/pod-sessions/:id", http.StatusOK, start)
	return &PodSessionResponse{PodSession: podSession}, nil
}

func (h *Handler) getPodSession(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/api/pod-sessions/:id", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authorizePodSessionPVC(ctx, principal, podSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodGet, "/api/pod-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	podSession, getErr := h.viewers.GetPodSession(podSessionID)
	if getErr != nil {
		apiErr := apienv.FromError(getErr)
		h.observe(ctx, http.MethodGet, "/api/pod-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/api/pod-sessions/:id", http.StatusOK, start)
	return &PodSessionResponse{PodSession: podSession}, nil
}

func (h *Handler) verifyFileBrowserHook(
	ctx context.Context,
	req *VerifyFileBrowserHookRequest,
) (*FileBrowserHookVerificationResponse, *apienv.Error) {
	start := time.Now()
	result := h.auth.VerifyHook(session.HookVerifyInput{
		HookClientToken: strings.TrimPrefix(req.Authorization, "Bearer "),
		PodSessionID:    req.PodSessionID,
		ViewerPodName:   req.ViewerPodName,
		Username:        req.Username,
		AuthRequestID:   req.AuthRequestID,
		PasswordHash:    req.PasswordHash,
	})
	h.observe(ctx, http.MethodPost, "/internal/filebrowser-hook/verify", http.StatusOK, start)
	return &FileBrowserHookVerificationResponse{FileBrowserHookVerification: result}, nil
}

func (h *Handler) observe(ctx context.Context, method string, route string, status int, start time.Time) {
	if h.recorder != nil {
		h.recorder.ObserveHTTP(ctx, method, route, status, time.Since(start))
	}
}

func authenticateRequest(req interface{ authorizationHeader() string }) (*authn.Principal, *apienv.Error) {
	principal, err := authn.PrincipalFromAuthorization(req.authorizationHeader())
	if err != nil {
		return nil, apienv.NewError(401, apienv.CodeUnauthorized, "Unauthorized", nil)
	}
	return principal, nil
}

func (req *AuthenticatedRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *ListPVCsRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *CreateViewerSessionRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func authenticatedRequest(req *http.Request) *AuthenticatedRequest {
	return &AuthenticatedRequest{Authorization: req.Header.Get("Authorization")}
}

func writeHTTPResponse(w http.ResponseWriter, response any, apiErr *apienv.Error) {
	if apiErr != nil {
		apienv.WriteError(w, apiErr)
		return
	}
	if headered, ok := response.(*ViewerTokenResponse); ok {
		w.Header().Set("Cache-Control", headered.CacheControl)
		w.Header().Set("Pragma", headered.Pragma)
	}
	status := httpStatus(response)
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(httpBody(response))
}

func httpStatus(response any) int {
	return 0
}

func httpBody(response any) any {
	switch typed := response.(type) {
	case *ViewerSessionResponse:
		return struct {
			ViewerSession *domain.ViewerSession `json:"viewer_session"`
		}{ViewerSession: typed.ViewerSession}
	case *ViewerTokenResponse:
		return struct {
			ViewerToken *domain.ViewerToken `json:"viewer_token"`
		}{ViewerToken: typed.ViewerToken}
	default:
		return response
	}
}

func toEncoreError(apiErr *apienv.Error) error {
	if apiErr == nil {
		return nil
	}
	return errs.B().
		Code(toEncoreErrorCode(apiErr.Status)).
		Msg(apiErr.Message).
		Details(ErrorDetails{Code: apiErr.Code}).
		Err()
}

func toEncoreErrorCode(status int) errs.ErrCode {
	switch status {
	case http.StatusBadRequest:
		return errs.InvalidArgument
	case http.StatusUnauthorized:
		return errs.Unauthenticated
	case http.StatusForbidden:
		return errs.PermissionDenied
	case http.StatusNotFound:
		return errs.NotFound
	case http.StatusConflict:
		return errs.Aborted
	case http.StatusServiceUnavailable:
		return errs.Unavailable
	default:
		return errs.Internal
	}
}

func metricsSnapshot(recorder *observability.Recorder) MetricsSnapshot {
	metrics := recorder.Metrics()
	return MetricsSnapshot{
		HTTPRequests:       metrics.HTTPRequests.Load(),
		HTTPErrors:         metrics.HTTPErrors.Load(),
		ViewerCreated:      metrics.ViewerCreated.Load(),
		ViewerClosed:       metrics.ViewerClosed.Load(),
		PodCreated:         metrics.PodCreated.Load(),
		PodReused:          metrics.PodReused.Load(),
		PodDeleted:         metrics.PodDeleted.Load(),
		AuthCreated:        metrics.AuthCreated.Load(),
		AuthConsumed:       metrics.AuthConsumed.Load(),
		AuthDenied:         metrics.AuthDenied.Load(),
		FileBrowserLogins:  metrics.FileBrowserLogins.Load(),
		FileBrowserErrors:  metrics.FileBrowserErrors.Load(),
		KubernetesRequests: metrics.KubernetesRequests.Load(),
		KubernetesErrors:   metrics.KubernetesErrors.Load(),
		CleanupDeleted:     metrics.CleanupDeleted.Load(),
	}
}

func (h *Handler) authorizeViewerSessionPVC(
	ctx context.Context,
	principal *authn.Principal,
	viewerSessionID string,
) error {
	session, err := h.viewers.GetViewerSession(ctx, viewerSessionID, principal.ID)
	if err != nil {
		return err
	}
	return h.authorizePodSessionPVC(ctx, principal, session.PodSessionID)
}

func (h *Handler) authorizePodSessionPVC(
	ctx context.Context,
	principal *authn.Principal,
	podSessionID string,
) error {
	podSession, err := h.viewers.GetPodSession(podSessionID)
	if err != nil {
		return err
	}
	if err := h.authz.CanGetPVC(ctx, principal, podSession.Namespace, podSession.PVCName); err != nil {
		return apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
	}
	return nil
}

func pathID(path string, prefix string) string {
	return strings.Trim(strings.TrimPrefix(path, prefix), "/")
}

var errRuntimeUnavailable = errors.New("viewer runtime is not configured")

type kubernetesAuthorizer struct {
	management kubernetes.Interface
}

func newKubernetesAuthorizer(management kubernetes.Interface) kubernetesAuthorizer {
	return kubernetesAuthorizer{management: management}
}

func (a kubernetesAuthorizer) CanListPVCs(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
) error {
	clientset, err := kubernetesClientsetForConfig(principal.ClientConfig)
	if err != nil {
		return err
	}
	if a.management != nil {
		if err := a.sameNamespace(ctx, clientset, namespace); err != nil {
			return err
		}
	}
	_, err = clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{Limit: 1})
	return err
}

func (a kubernetesAuthorizer) CanGetPVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
	name string,
) error {
	clientset, err := kubernetesClientsetForConfig(principal.ClientConfig)
	if err != nil {
		return err
	}
	userPVC, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if a.management == nil {
		return nil
	}
	managedPVC, err := a.management.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if userPVC.UID != managedPVC.UID {
		return errors.New("user kubeconfig and management kubeconfig resolved different PVCs")
	}
	return nil
}

func (a kubernetesAuthorizer) sameNamespace(
	ctx context.Context,
	userClient kubernetes.Interface,
	namespace string,
) error {
	userNamespace, err := userClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}
	managedNamespace, err := a.management.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if userNamespace.UID != managedNamespace.UID {
		return errors.New("user kubeconfig and management kubeconfig resolved different namespaces")
	}
	return nil
}
