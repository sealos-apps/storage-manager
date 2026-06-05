package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"encore.dev/beta/errs"
	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var kubernetesClientsetForConfig = func(c *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(c)
}

type viewerService interface {
	ListNamespaces(ctx context.Context) ([]corev1.Namespace, error)
	ListPVCs(ctx context.Context, namespace string) ([]domain.PVC, error)
	CreatePVC(ctx context.Context, input session.CreatePVCInput) (*domain.PVC, error)
	DeletePVC(ctx context.Context, input session.DeletePVCInput) (*domain.PVC, error)
	ExpandPVC(ctx context.Context, input session.ExpandPVCInput) (*domain.PVC, error)
	ListStorageClasses(ctx context.Context) ([]domain.StorageClass, error)
	CreateViewerSession(ctx context.Context, input session.CreateViewerSessionInput) (*domain.ViewerSession, error)
	GetViewerSession(ctx context.Context, id string, userID string) (*domain.ViewerSession, error)
	IssueToken(ctx context.Context, id string, userID string) (*domain.ViewerToken, error)
	HeartbeatForUser(ctx context.Context, id string, userID string) (*domain.Heartbeat, error)
	CloseViewerSessionForUser(id string, userID string) (*domain.ViewerSession, error)
	GetPodSession(id string) (*domain.PodSession, error)
}

type storageClassService interface {
	ListStorageClasses(ctx context.Context, includeHidden bool) ([]domain.StorageClass, error)
	GetStorageClassYAML(ctx context.Context, name string) (*session.StorageClassYAML, error)
	CreateStorageClass(ctx context.Context, body string) (*domain.StorageClass, error)
	UpdateStorageClass(ctx context.Context, name string, body string) (*domain.StorageClass, error)
	UpdateStorageClassPolicy(
		ctx context.Context,
		name string,
		input session.StorageClassPolicyInput,
	) (*domain.StorageClass, error)
	DeleteStorageClass(ctx context.Context, name string) (*domain.StorageClass, error)
	DescribeStorageClass(ctx context.Context, name string) (*session.StorageClassDescribe, error)
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
	CanCreatePVC(ctx context.Context, principal *authn.Principal, namespace string) error
	CanDeletePVC(ctx context.Context, principal *authn.Principal, namespace string, name string) error
	CanUpdatePVC(ctx context.Context, principal *authn.Principal, namespace string, name string) error
	CanListStorageClasses(ctx context.Context, principal *authn.Principal) error
}

type adminAuthorizer interface {
	CanAdmin(ctx context.Context, principal *authn.Principal) (AdminAuthorizationResult, error)
	CanManageStorageClasses(ctx context.Context, principal *authn.Principal) error
}

type Handler struct {
	viewers        viewerService
	storageClasses storageClassService
	pods           podService
	auth           authService
	recorder       *observability.Recorder
	authz          authorizer
	adminAuthz     adminAuthorizer
	debug          config.DebugConfig
}

type HandlerOption func(*Handler)

type operationMode string

const (
	operationModeUser  operationMode = "user"
	operationModeAdmin operationMode = "admin"
)

type operationContext struct {
	admin             AdminAuthorizationResult
	adminContext      bool
	implicitElevation bool
	kubeService       viewerService
	mode              operationMode
	namespace         string
	namespaceAllowed  bool
	principal         *authn.Principal
}

type auditDecision struct {
	adminAllowed       bool
	authorizationKind  string
	decision           string
	denyReason         string
	executionKind      string
	identityMethod     string
	implicitElevation  bool
	kubernetesUsername string
	mode               operationMode
	namespace          string
	namespaceAllowed   bool
	podSessionID       string
	principal          *authn.Principal
	pvcName            string
	route              string
	viewerSessionID    string
}

func WithDebugConfig(debug config.DebugConfig) HandlerOption {
	return func(h *Handler) {
		h.debug = debug
	}
}

func WithStorageClassService(storageClasses storageClassService) HandlerOption {
	return func(h *Handler) {
		h.storageClasses = storageClasses
	}
}

func WithAdminAuthorizer(authz adminAuthorizer) HandlerOption {
	return func(h *Handler) {
		h.adminAuthz = authz
	}
}

type AuthenticatedRequest struct {
	Authorization string `header:"Authorization" encore:"sensitive"`
}

type ListPVCsRequest struct {
	Authorization string `header:"Authorization" encore:"sensitive"`
	Namespace     string `query:"namespace"`
}

type CreatePVCRequest struct {
	Authorization    string   `header:"Authorization" encore:"sensitive"`
	Namespace        string   `json:"namespace"`
	Name             string   `json:"name"`
	Capacity         string   `json:"capacity"`
	CapacityBytes    int64    `json:"capacity_bytes"`
	AccessModes      []string `json:"access_modes"`
	StorageClassName string   `json:"storage_class_name"`
}

type ExpandPVCRequest struct {
	Authorization string `header:"Authorization" encore:"sensitive"`
	Capacity      string `json:"capacity"`
	CapacityBytes int64  `json:"capacity_bytes"`
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

type NamespaceList struct {
	Items []domain.Namespace `json:"items"`
}

type ListNamespacesResponse struct {
	NamespaceList NamespaceList `json:"namespace_list"`
}

type ViewerContext struct {
	ContextName string `json:"context_name"`
	Namespace   string `json:"namespace"`
}

type ContextResponse struct {
	Context ViewerContext `json:"context"`
}

type PVCResponse struct {
	PVC *domain.PVC `json:"pvc"`
}

type StorageClassList struct {
	Items []domain.StorageClass `json:"items"`
}

type ListStorageClassesResponse struct {
	StorageClassList StorageClassList `json:"storage_class_list"`
}

type AdminCapabilitySet struct {
	CanManagePVCs           bool `json:"can_manage_pvcs"`
	CanManageStorageClasses bool `json:"can_manage_storage_classes"`
}

type AdminCapabilitiesResponse struct {
	AdminCapabilities AdminCapabilitySet `json:"admin_capabilities"`
}

type StorageClassResponse struct {
	StorageClass *domain.StorageClass `json:"storage_class"`
}

type StorageClassYAMLResponse struct {
	StorageClassYAML *session.StorageClassYAML `json:"storage_class_yaml"`
}

type StorageClassYAMLRequest struct {
	Authorization string `header:"Authorization" encore:"sensitive"`
	YAML          string `json:"yaml"`
}

type StorageClassPolicyRequest struct {
	Authorization      string   `header:"Authorization" encore:"sensitive"`
	VisibleInCreate    bool     `json:"visible_in_create"`
	AllowedAccessModes []string `json:"allowed_access_modes"`
}

type StorageClassDescribeResponse struct {
	StorageClassDescribe *session.StorageClassDescribe `json:"storage_class_describe"`
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

type ErrorDetails struct {
	Code    apienv.Code    `json:"code"`
	Details map[string]any `json:"details,omitempty"`
	Message string         `json:"message,omitempty"`
}

func (ErrorDetails) ErrDetails() {}

func NewHandler(
	viewers viewerService,
	pods podService,
	auth authService,
	managementClient kubernetes.Interface,
	recorder *observability.Recorder,
	authz authorizer,
	options ...HandlerOption,
) *Handler {
	if authz == nil {
		authz = newKubernetesAuthorizer(managementClient, recorder)
	}
	handler := &Handler{
		viewers:        viewers,
		storageClasses: unavailableStorageClassService{},
		pods:           pods,
		auth:           auth,
		recorder:       recorder,
		authz:          authz,
		adminAuthz:     denyAdminAuthorizer{},
	}
	for _, option := range options {
		option(handler)
	}
	return handler
}

func (h *Handler) ListPVCsData(ctx context.Context, req *ListPVCsRequest) (*ListPVCsResponse, error) {
	response, apiErr := h.listPVCs(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) GetContextData(ctx context.Context, req *AuthenticatedRequest) (*ContextResponse, error) {
	response, apiErr := h.getContext(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) CreatePVCData(ctx context.Context, req *CreatePVCRequest) (*PVCResponse, error) {
	response, apiErr := h.createPVC(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) DeletePVCData(
	ctx context.Context,
	namespace string,
	name string,
	req *AuthenticatedRequest,
) (*PVCResponse, error) {
	response, apiErr := h.deletePVC(ctx, namespace, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) ExpandPVCData(
	ctx context.Context,
	namespace string,
	name string,
	req *ExpandPVCRequest,
) (*PVCResponse, error) {
	response, apiErr := h.expandPVC(ctx, namespace, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) ListStorageClassesData(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListStorageClassesResponse, error) {
	response, apiErr := h.listStorageClasses(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminCapabilitiesData(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*AdminCapabilitiesResponse, error) {
	response, apiErr := h.adminCapabilities(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminListNamespacesData(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListNamespacesResponse, error) {
	response, apiErr := h.adminListNamespaces(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminListStorageClassesData(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListStorageClassesResponse, error) {
	response, apiErr := h.adminListStorageClasses(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminGetStorageClassYAMLData(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassYAMLResponse, error) {
	response, apiErr := h.adminGetStorageClassYAML(ctx, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminCreateStorageClassData(
	ctx context.Context,
	req *StorageClassYAMLRequest,
) (*StorageClassResponse, error) {
	response, apiErr := h.adminCreateStorageClass(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminUpdateStorageClassData(
	ctx context.Context,
	name string,
	req *StorageClassYAMLRequest,
) (*StorageClassResponse, error) {
	response, apiErr := h.adminUpdateStorageClass(ctx, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminUpdateStorageClassPolicyData(
	ctx context.Context,
	name string,
	req *StorageClassPolicyRequest,
) (*StorageClassResponse, error) {
	response, apiErr := h.adminUpdateStorageClassPolicy(ctx, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminDeleteStorageClassData(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassResponse, error) {
	response, apiErr := h.adminDeleteStorageClass(ctx, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminDescribeStorageClassData(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassDescribeResponse, error) {
	response, apiErr := h.adminDescribeStorageClass(ctx, name, req)
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

func (h *Handler) ListPVCs(w http.ResponseWriter, req *http.Request) {
	response, err := h.listPVCs(req.Context(), &ListPVCsRequest{
		Authorization: req.Header.Get("Authorization"),
		Namespace:     req.URL.Query().Get("namespace"),
	})
	writeHTTPResponse(w, response, err)
}

func (h *Handler) GetContext(w http.ResponseWriter, req *http.Request) {
	response, err := h.getContext(req.Context(), authenticatedRequest(req))
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

func (h *Handler) CreatePVC(w http.ResponseWriter, req *http.Request) {
	var body CreatePVCRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeHTTPResponse(w, nil, apienv.NewError(400, apienv.CodeValidationError, "Invalid JSON request", nil))
		return
	}
	body.Authorization = req.Header.Get("Authorization")
	response, err := h.createPVC(req.Context(), &body)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) DeletePVC(w http.ResponseWriter, req *http.Request) {
	namespace, name := pvcPathParams(req.URL.Path)
	response, err := h.deletePVC(req.Context(), namespace, name, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) ExpandPVC(w http.ResponseWriter, req *http.Request) {
	var body ExpandPVCRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeHTTPResponse(w, nil, apienv.NewError(400, apienv.CodeValidationError, "Invalid JSON request", nil))
		return
	}
	body.Authorization = req.Header.Get("Authorization")
	namespace, name := expandPVCPathParams(req.URL.Path)
	response, err := h.expandPVC(req.Context(), namespace, name, &body)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) ListStorageClasses(w http.ResponseWriter, req *http.Request) {
	response, err := h.listStorageClasses(req.Context(), authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminCapabilities(w http.ResponseWriter, req *http.Request) {
	response, err := h.adminCapabilities(req.Context(), authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminListNamespaces(w http.ResponseWriter, req *http.Request) {
	response, err := h.adminListNamespaces(req.Context(), authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminListStorageClasses(w http.ResponseWriter, req *http.Request) {
	response, err := h.adminListStorageClasses(req.Context(), authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminGetStorageClassYAML(w http.ResponseWriter, req *http.Request) {
	name := strings.TrimSuffix(pathID(req.URL.Path, "/api/admin/storage-classes/"), "/yaml")
	response, err := h.adminGetStorageClassYAML(req.Context(), name, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminCreateStorageClass(w http.ResponseWriter, req *http.Request) {
	var body StorageClassYAMLRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeHTTPResponse(w, nil, apienv.NewError(400, apienv.CodeValidationError, "Invalid JSON request", nil))
		return
	}
	body.Authorization = req.Header.Get("Authorization")
	response, err := h.adminCreateStorageClass(req.Context(), &body)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminUpdateStorageClass(w http.ResponseWriter, req *http.Request) {
	var body StorageClassYAMLRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeHTTPResponse(w, nil, apienv.NewError(400, apienv.CodeValidationError, "Invalid JSON request", nil))
		return
	}
	body.Authorization = req.Header.Get("Authorization")
	name := pathID(req.URL.Path, "/api/admin/storage-classes/")
	response, err := h.adminUpdateStorageClass(req.Context(), name, &body)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminUpdateStorageClassPolicy(w http.ResponseWriter, req *http.Request) {
	var body StorageClassPolicyRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeHTTPResponse(w, nil, apienv.NewError(400, apienv.CodeValidationError, "Invalid JSON request", nil))
		return
	}
	body.Authorization = req.Header.Get("Authorization")
	name := strings.TrimSuffix(pathID(req.URL.Path, "/api/admin/storage-classes/"), "/policy")
	response, err := h.adminUpdateStorageClassPolicy(req.Context(), name, &body)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminDeleteStorageClass(w http.ResponseWriter, req *http.Request) {
	name := pathID(req.URL.Path, "/api/admin/storage-classes/")
	response, err := h.adminDeleteStorageClass(req.Context(), name, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminDescribeStorageClass(w http.ResponseWriter, req *http.Request) {
	name := strings.TrimSuffix(pathID(req.URL.Path, "/api/admin/storage-classes/"), "/describe")
	response, err := h.adminDescribeStorageClass(req.Context(), name, authenticatedRequest(req))
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
	h.recorder.WritePrometheus(w, req)
}

func (h *Handler) listPVCs(ctx context.Context, req *ListPVCsRequest) (*ListPVCsResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/api/pvcs", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, req.Namespace, "/api/pvcs", "")
	if apiErr != nil {
		h.observe(ctx, http.MethodGet, "/api/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanListPVCs(ctx, principal, op.namespace); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
			h.recordAudit(ctx, auditDecision{
				adminAllowed:      false,
				decision:          "deny",
				denyReason:        "ssar_denied",
				implicitElevation: false,
				mode:              operationModeUser,
				namespace:         op.namespace,
				namespaceAllowed:  true,
				principal:         principal,
				route:             "/api/pvcs",
			})
			h.observe(ctx, http.MethodGet, "/api/pvcs", apiErr.Status, start)
			return nil, apiErr
		}
	}
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       op.mode == operationModeAdmin,
		decision:           "allow",
		identityMethod:     identityMethodForOperation(op),
		implicitElevation:  op.implicitElevation,
		kubernetesUsername: op.admin.KubernetesUsername,
		mode:               op.mode,
		namespace:          op.namespace,
		namespaceAllowed:   op.namespaceAllowed,
		principal:          principal,
		route:              "/api/pvcs",
	})
	items, listErr := op.kubeService.ListPVCs(ctx, op.namespace)
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

func (h *Handler) getContext(ctx context.Context, req *AuthenticatedRequest) (*ContextResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/api/context", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authz.CanListPVCs(ctx, principal, principal.Namespace); err != nil {
		apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "Namespace access denied", nil)
		h.observe(ctx, http.MethodGet, "/api/context", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/api/context", http.StatusOK, start)
	return &ContextResponse{
		Context: ViewerContext{
			ContextName: principal.ContextName,
			Namespace:   principal.Namespace,
		},
	}, nil
}

func (h *Handler) createPVC(ctx context.Context, req *CreatePVCRequest) (*PVCResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPost, "/api/pvcs", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, req.Namespace, "/api/pvcs", req.Name)
	if apiErr != nil {
		h.observe(ctx, http.MethodPost, "/api/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanCreatePVC(ctx, principal, op.namespace); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCCreateForbidden, "PVC create access denied", nil)
			h.recordAudit(ctx, auditDecision{
				decision:         "deny",
				denyReason:       "ssar_denied",
				mode:             operationModeUser,
				namespace:        op.namespace,
				namespaceAllowed: true,
				principal:        principal,
				pvcName:          req.Name,
				route:            "/api/pvcs",
			})
			h.observe(ctx, http.MethodPost, "/api/pvcs", apiErr.Status, start)
			return nil, apiErr
		}
	}
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       op.mode == operationModeAdmin,
		decision:           "allow",
		identityMethod:     identityMethodForOperation(op),
		implicitElevation:  op.implicitElevation,
		kubernetesUsername: op.admin.KubernetesUsername,
		mode:               op.mode,
		namespace:          op.namespace,
		namespaceAllowed:   op.namespaceAllowed,
		principal:          principal,
		pvcName:            req.Name,
		route:              "/api/pvcs",
	})
	pvc, createErr := op.kubeService.CreatePVC(ctx, session.CreatePVCInput{
		Namespace:        op.namespace,
		Name:             req.Name,
		Capacity:         req.Capacity,
		CapacityBytes:    req.CapacityBytes,
		AccessModes:      req.AccessModes,
		StorageClassName: req.StorageClassName,
	})
	if createErr != nil {
		apiErr := apienv.FromError(createErr)
		h.observe(ctx, http.MethodPost, "/api/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/api/pvcs", http.StatusCreated, start)
	return &PVCResponse{PVC: pvc}, nil
}

func (h *Handler) deletePVC(
	ctx context.Context,
	namespace string,
	name string,
	req *AuthenticatedRequest,
) (*PVCResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodDelete, "/api/pvcs/:namespace/:name", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, namespace, "/api/pvcs/:namespace/:name", name)
	if apiErr != nil {
		h.observe(ctx, http.MethodDelete, "/api/pvcs/:namespace/:name", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanDeletePVC(ctx, principal, op.namespace, name); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCDeleteForbidden, "PVC delete access denied", nil)
			h.recordAudit(ctx, auditDecision{
				decision:         "deny",
				denyReason:       "ssar_denied",
				mode:             operationModeUser,
				namespace:        op.namespace,
				namespaceAllowed: true,
				principal:        principal,
				pvcName:          name,
				route:            "/api/pvcs/:namespace/:name",
			})
			h.observe(ctx, http.MethodDelete, "/api/pvcs/:namespace/:name", apiErr.Status, start)
			return nil, apiErr
		}
	}
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       op.mode == operationModeAdmin,
		decision:           "allow",
		identityMethod:     identityMethodForOperation(op),
		implicitElevation:  op.implicitElevation,
		kubernetesUsername: op.admin.KubernetesUsername,
		mode:               op.mode,
		namespace:          op.namespace,
		namespaceAllowed:   op.namespaceAllowed,
		principal:          principal,
		pvcName:            name,
		route:              "/api/pvcs/:namespace/:name",
	})
	pvc, deleteErr := op.kubeService.DeletePVC(ctx, session.DeletePVCInput{
		Namespace: op.namespace,
		Name:      name,
	})
	if deleteErr != nil {
		apiErr := apienv.FromError(deleteErr)
		h.observe(ctx, http.MethodDelete, "/api/pvcs/:namespace/:name", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodDelete, "/api/pvcs/:namespace/:name", http.StatusOK, start)
	return &PVCResponse{PVC: pvc}, nil
}

func (h *Handler) expandPVC(
	ctx context.Context,
	namespace string,
	name string,
	req *ExpandPVCRequest,
) (*PVCResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPost, "/api/pvcs/:namespace/:name/expand", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, namespace, "/api/pvcs/:namespace/:name/expand", name)
	if apiErr != nil {
		h.observe(ctx, http.MethodPost, "/api/pvcs/:namespace/:name/expand", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanUpdatePVC(ctx, principal, op.namespace, name); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCExpandForbidden, "PVC expand access denied", nil)
			h.recordAudit(ctx, auditDecision{
				decision:         "deny",
				denyReason:       "ssar_denied",
				mode:             operationModeUser,
				namespace:        op.namespace,
				namespaceAllowed: true,
				principal:        principal,
				pvcName:          name,
				route:            "/api/pvcs/:namespace/:name/expand",
			})
			h.observe(ctx, http.MethodPost, "/api/pvcs/:namespace/:name/expand", apiErr.Status, start)
			return nil, apiErr
		}
	}
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       op.mode == operationModeAdmin,
		decision:           "allow",
		identityMethod:     identityMethodForOperation(op),
		implicitElevation:  op.implicitElevation,
		kubernetesUsername: op.admin.KubernetesUsername,
		mode:               op.mode,
		namespace:          op.namespace,
		namespaceAllowed:   op.namespaceAllowed,
		principal:          principal,
		pvcName:            name,
		route:              "/api/pvcs/:namespace/:name/expand",
	})
	pvc, expandErr := op.kubeService.ExpandPVC(ctx, session.ExpandPVCInput{
		Namespace:     op.namespace,
		Name:          name,
		Capacity:      req.Capacity,
		CapacityBytes: req.CapacityBytes,
	})
	if expandErr != nil {
		apiErr := apienv.FromError(expandErr)
		h.observe(ctx, http.MethodPost, "/api/pvcs/:namespace/:name/expand", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/api/pvcs/:namespace/:name/expand", http.StatusOK, start)
	return &PVCResponse{PVC: pvc}, nil
}

func (h *Handler) listStorageClasses(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListStorageClassesResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/api/storage-classes", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authz.CanListStorageClasses(ctx, principal); err != nil {
		apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "Storage class access denied", nil)
		h.observe(ctx, http.MethodGet, "/api/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	items, listErr := h.viewers.ListStorageClasses(ctx)
	if listErr != nil {
		apiErr := apienv.FromError(listErr)
		h.observe(ctx, http.MethodGet, "/api/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	if items == nil {
		items = []domain.StorageClass{}
	}
	h.observe(ctx, http.MethodGet, "/api/storage-classes", http.StatusOK, start)
	return &ListStorageClassesResponse{StorageClassList: StorageClassList{Items: items}}, nil
}

func (h *Handler) adminCapabilities(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*AdminCapabilitiesResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/api/admin/capabilities", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	adminResult, adminErr := h.checkAdmin(ctx, principal)
	canManage := adminErr == nil && adminResult.Allowed
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       canManage,
		authorizationKind:  "kubeconfig",
		decision:           allowDeny(canManage),
		denyReason:         denyReason(adminErr, adminResult.Reason),
		executionKind:      "management_service_account",
		identityMethod:     "kubeconfig_context+self_subject_review",
		kubernetesUsername: adminResult.KubernetesUsername,
		mode:               operationModeAdmin,
		namespace:          principal.Namespace,
		namespaceAllowed:   true,
		principal:          principal,
		route:              "/api/admin/capabilities",
	})
	h.observe(ctx, http.MethodGet, "/api/admin/capabilities", http.StatusOK, start)
	return &AdminCapabilitiesResponse{
		AdminCapabilities: AdminCapabilitySet{
			CanManagePVCs:           canManage,
			CanManageStorageClasses: canManage,
		},
	}, nil
}

func (h *Handler) adminListNamespaces(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListNamespacesResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/api/admin/namespaces", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	adminResult, adminErr := h.checkAdmin(ctx, principal)
	if adminErr != nil {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       false,
			authorizationKind:  "kubeconfig",
			decision:           "deny",
			denyReason:         denyReason(adminErr, adminResult.Reason),
			executionKind:      "management_service_account",
			identityMethod:     "kubeconfig_context+self_subject_review",
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          principal.Namespace,
			namespaceAllowed:   false,
			principal:          principal,
			route:              "/api/admin/namespaces",
		})
		apiErr := apienv.NewError(403, apienv.CodeAdminAccessDenied, "Admin access denied", nil)
		h.observe(ctx, http.MethodGet, "/api/admin/namespaces", apiErr.Status, start)
		return nil, apiErr
	}
	namespaces, listErr := h.viewers.ListNamespaces(ctx)
	if listErr != nil {
		apiErr := apienv.FromError(listErr)
		h.observe(ctx, http.MethodGet, "/api/admin/namespaces", apiErr.Status, start)
		return nil, apiErr
	}
	items := allowedAdminNamespaces(namespaces, principal.Namespace)
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       true,
		authorizationKind:  "kubeconfig",
		decision:           "allow",
		executionKind:      "management_service_account",
		identityMethod:     "kubeconfig_context+self_subject_review",
		kubernetesUsername: adminResult.KubernetesUsername,
		mode:               operationModeAdmin,
		namespace:          principal.Namespace,
		namespaceAllowed:   true,
		principal:          principal,
		route:              "/api/admin/namespaces",
	})
	h.observe(ctx, http.MethodGet, "/api/admin/namespaces", http.StatusOK, start)
	return &ListNamespacesResponse{NamespaceList: NamespaceList{Items: items}}, nil
}

func (h *Handler) adminListStorageClasses(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListStorageClassesResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodGet, "/api/admin/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	items, listErr := h.storageClasses.ListStorageClasses(ctx, true)
	if listErr != nil {
		apiErr := apienv.FromError(listErr)
		h.observe(ctx, http.MethodGet, "/api/admin/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	if items == nil {
		items = []domain.StorageClass{}
	}
	h.observe(ctx, http.MethodGet, "/api/admin/storage-classes", http.StatusOK, start)
	return &ListStorageClassesResponse{StorageClassList: StorageClassList{Items: items}}, nil
}

func (h *Handler) adminGetStorageClassYAML(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassYAMLResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodGet, "/api/admin/storage-classes/:name/yaml", apiErr.Status, start)
		return nil, apiErr
	}
	result, getErr := h.storageClasses.GetStorageClassYAML(ctx, name)
	if getErr != nil {
		apiErr := apienv.FromError(getErr)
		h.observe(ctx, http.MethodGet, "/api/admin/storage-classes/:name/yaml", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/api/admin/storage-classes/:name/yaml", http.StatusOK, start)
	return &StorageClassYAMLResponse{StorageClassYAML: result}, nil
}

func (h *Handler) adminCreateStorageClass(
	ctx context.Context,
	req *StorageClassYAMLRequest,
) (*StorageClassResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodPost, "/api/admin/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	item, createErr := h.storageClasses.CreateStorageClass(ctx, req.YAML)
	if createErr != nil {
		apiErr := apienv.FromError(createErr)
		h.observe(ctx, http.MethodPost, "/api/admin/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/api/admin/storage-classes", http.StatusCreated, start)
	return &StorageClassResponse{StorageClass: item}, nil
}

func (h *Handler) adminUpdateStorageClass(
	ctx context.Context,
	name string,
	req *StorageClassYAMLRequest,
) (*StorageClassResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodPut, "/api/admin/storage-classes/:name", apiErr.Status, start)
		return nil, apiErr
	}
	item, updateErr := h.storageClasses.UpdateStorageClass(ctx, name, req.YAML)
	if updateErr != nil {
		apiErr := apienv.FromError(updateErr)
		h.observe(ctx, http.MethodPut, "/api/admin/storage-classes/:name", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPut, "/api/admin/storage-classes/:name", http.StatusOK, start)
	return &StorageClassResponse{StorageClass: item}, nil
}

func (h *Handler) adminUpdateStorageClassPolicy(
	ctx context.Context,
	name string,
	req *StorageClassPolicyRequest,
) (*StorageClassResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodPut, "/api/admin/storage-classes/:name/policy", apiErr.Status, start)
		return nil, apiErr
	}
	item, updateErr := h.storageClasses.UpdateStorageClassPolicy(
		ctx,
		name,
		session.StorageClassPolicyInput{
			AllowedAccessModes: req.AllowedAccessModes,
			VisibleInCreate:    req.VisibleInCreate,
		},
	)
	if updateErr != nil {
		apiErr := apienv.FromError(updateErr)
		h.observe(ctx, http.MethodPut, "/api/admin/storage-classes/:name/policy", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPut, "/api/admin/storage-classes/:name/policy", http.StatusOK, start)
	return &StorageClassResponse{StorageClass: item}, nil
}

func (h *Handler) adminDeleteStorageClass(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodDelete, "/api/admin/storage-classes/:name", apiErr.Status, start)
		return nil, apiErr
	}
	item, deleteErr := h.storageClasses.DeleteStorageClass(ctx, name)
	if deleteErr != nil {
		apiErr := apienv.FromError(deleteErr)
		h.observe(ctx, http.MethodDelete, "/api/admin/storage-classes/:name", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodDelete, "/api/admin/storage-classes/:name", http.StatusOK, start)
	return &StorageClassResponse{StorageClass: item}, nil
}

func (h *Handler) adminDescribeStorageClass(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassDescribeResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodGet, "/api/admin/storage-classes/:name/describe", apiErr.Status, start)
		return nil, apiErr
	}
	result, describeErr := h.storageClasses.DescribeStorageClass(ctx, name)
	if describeErr != nil {
		apiErr := apienv.FromError(describeErr)
		h.observe(ctx, http.MethodGet, "/api/admin/storage-classes/:name/describe", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/api/admin/storage-classes/:name/describe", http.StatusOK, start)
	return &StorageClassDescribeResponse{StorageClassDescribe: result}, nil
}

func (h *Handler) authorizeStorageClassAdmin(
	ctx context.Context,
	req interface{ authorizationHeader() string },
) *apienv.Error {
	principal, err := h.authenticateRequest(req)
	if err != nil {
		return err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	adminResult, adminErr := h.checkAdmin(ctx, principal)
	if adminErr != nil {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       false,
			decision:           "deny",
			denyReason:         denyReason(adminErr, adminResult.Reason),
			identityMethod:     "kubeconfig_context+self_subject_review",
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          principal.Namespace,
			namespaceAllowed:   true,
			principal:          principal,
			route:              "/api/admin/storage-classes",
		})
		return apienv.NewError(403, apienv.CodeAdminAccessDenied, "Admin access denied", nil)
	}
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       true,
		decision:           "allow",
		identityMethod:     "kubeconfig_context+self_subject_review",
		kubernetesUsername: adminResult.KubernetesUsername,
		mode:               operationModeAdmin,
		namespace:          principal.Namespace,
		namespaceAllowed:   true,
		principal:          principal,
		route:              "/api/admin/storage-classes",
	})
	return nil
}

func (h *Handler) createViewerSession(
	ctx context.Context,
	req *CreateViewerSessionRequest,
) (*ViewerSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if req.PVCName == "" {
		apiErr := apienv.NewError(400, apienv.CodeValidationError, "pvc_name is required", nil)
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions", apiErr.Status, start)
		return nil, apiErr
	}
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, req.Namespace, "/api/viewer-sessions", req.PVCName)
	if apiErr != nil {
		h.observe(ctx, http.MethodPost, "/api/viewer-sessions", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanGetPVC(ctx, principal, op.namespace, req.PVCName); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
			h.recordAudit(ctx, auditDecision{
				decision:         "deny",
				denyReason:       "ssar_denied",
				mode:             operationModeUser,
				namespace:        op.namespace,
				namespaceAllowed: true,
				principal:        principal,
				pvcName:          req.PVCName,
				route:            "/api/viewer-sessions",
			})
			h.observe(ctx, http.MethodPost, "/api/viewer-sessions", apiErr.Status, start)
			return nil, apiErr
		}
	}
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       op.mode == operationModeAdmin,
		decision:           "allow",
		identityMethod:     identityMethodForOperation(op),
		implicitElevation:  op.implicitElevation,
		kubernetesUsername: op.admin.KubernetesUsername,
		mode:               op.mode,
		namespace:          op.namespace,
		namespaceAllowed:   op.namespaceAllowed,
		principal:          principal,
		pvcName:            req.PVCName,
		route:              "/api/viewer-sessions",
	})
	viewerSession, createErr := op.kubeService.CreateViewerSession(ctx, session.CreateViewerSessionInput{
		AdminContext: op.adminContext,
		Namespace:    op.namespace,
		PVCName:      req.PVCName,
		UserID:       principal.ID,
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

func requirePrincipalNamespace(namespace string, principal *authn.Principal) *apienv.Error {
	if namespace == "" || namespace == principal.Namespace {
		return nil
	}
	return apienv.NewError(403, apienv.CodePVCAccessDenied, "Namespace must match kubeconfig context namespace", nil)
}

func (h *Handler) resolvePVCOperationContext(
	ctx context.Context,
	principal *authn.Principal,
	requestedNamespace string,
	route string,
	pvcName string,
) (*operationContext, *apienv.Error) {
	namespace := strings.TrimSpace(requestedNamespace)
	if namespace == "" {
		namespace = principal.Namespace
	}
	if namespace == principal.Namespace {
		return &operationContext{
			kubeService:      h.viewers,
			mode:             operationModeUser,
			namespace:        namespace,
			namespaceAllowed: true,
			principal:        principal,
		}, nil
	}
	adminResult, adminErr := h.checkAdmin(ctx, principal)
	if adminErr != nil {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       false,
			authorizationKind:  "kubeconfig",
			decision:           "deny",
			denyReason:         denyReason(adminErr, adminResult.Reason),
			executionKind:      "management_service_account",
			identityMethod:     "kubeconfig_context+self_subject_review",
			implicitElevation:  true,
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          namespace,
			namespaceAllowed:   false,
			principal:          principal,
			pvcName:            pvcName,
			route:              route,
		})
		return nil, apienv.NewError(403, apienv.CodeAdminAccessDenied, "Admin access denied", nil)
	}
	namespaces, err := h.viewers.ListNamespaces(ctx)
	if err != nil {
		return nil, apienv.FromError(err)
	}
	namespaceAllowed := isAdminNamespaceAllowed(namespaces, principal.Namespace, namespace)
	if !namespaceAllowed {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       true,
			authorizationKind:  "kubeconfig",
			decision:           "deny",
			denyReason:         "namespace_not_allowed",
			executionKind:      "management_service_account",
			identityMethod:     "kubeconfig_context+self_subject_review",
			implicitElevation:  true,
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          namespace,
			namespaceAllowed:   false,
			principal:          principal,
			pvcName:            pvcName,
			route:              route,
		})
		return nil, apienv.NewError(403, apienv.CodePVCAccessDenied, "Namespace is not allowed for admin PVC access", nil)
	}
	return &operationContext{
		admin:             adminResult,
		adminContext:      true,
		implicitElevation: true,
		kubeService:       h.viewers,
		mode:              operationModeAdmin,
		namespace:         namespace,
		namespaceAllowed:  true,
		principal:         principal,
	}, nil
}

func (h *Handler) checkAdmin(ctx context.Context, principal *authn.Principal) (AdminAuthorizationResult, error) {
	if h.adminAuthz == nil {
		return AdminAuthorizationResult{Reason: "not_configured"}, errors.New("admin access denied")
	}
	return h.adminAuthz.CanAdmin(ctx, principal)
}

func allowedAdminNamespaces(namespaces []corev1.Namespace, currentNamespace string) []domain.Namespace {
	items := make([]domain.Namespace, 0, len(namespaces)+1)
	seen := map[string]struct{}{}
	currentNamespace = strings.TrimSpace(currentNamespace)
	if currentNamespace != "" {
		seen[currentNamespace] = struct{}{}
		items = append(items, domain.Namespace{Name: currentNamespace, IsCurrentContext: true})
	}
	for _, namespace := range namespaces {
		name := strings.TrimSpace(namespace.Name)
		if name == "" {
			continue
		}
		// Sealos user namespaces use the ns- prefix. Admin PVC browsing only exposes
		// system namespaces so the dropdown stays bounded and never lists other users.
		if strings.HasPrefix(name, "ns-") {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		items = append(items, domain.Namespace{Name: name, IsCurrentContext: name == currentNamespace})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsCurrentContext != items[j].IsCurrentContext {
			return items[i].IsCurrentContext
		}
		return items[i].Name < items[j].Name
	})
	return items
}

func isAdminNamespaceAllowed(namespaces []corev1.Namespace, currentNamespace string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	return slices.ContainsFunc(allowedAdminNamespaces(namespaces, currentNamespace), func(namespace domain.Namespace) bool {
		return namespace.Name == target
	})
}

func (h *Handler) getViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
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
	principal, err := h.authenticateRequest(req)
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
	principal, err := h.authenticateRequest(req)
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
	heartbeat, heartbeatErr := h.viewers.HeartbeatForUser(ctx, viewerSessionID, principal.ID)
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
	principal, err := h.authenticateRequest(req)
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
	principal, err := h.authenticateRequest(req)
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
	principal, err := h.authenticateRequest(req)
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
	ctx, finish := h.recorder.TraceOperation(ctx,
		"filebrowser.verify_hook",
		slog.String("pod_session_id", req.PodSessionID),
		slog.String("viewer_pod_name", req.ViewerPodName),
	)
	defer finish(nil)

	result := h.auth.VerifyHook(session.HookVerifyInput{
		HookClientToken: strings.TrimPrefix(req.Authorization, "Bearer "),
		PodSessionID:    req.PodSessionID,
		ViewerPodName:   req.ViewerPodName,
		Username:        req.Username,
		AuthRequestID:   req.AuthRequestID,
		PasswordHash:    req.PasswordHash,
	})
	h.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "filebrowser.hook_verified",
		slog.String("pod_session_id", req.PodSessionID),
		slog.Bool("allowed", result.Allow),
		slog.String("reason", result.Reason),
	)
	h.observe(ctx, http.MethodPost, "/internal/filebrowser-hook/verify", http.StatusOK, start)
	return &FileBrowserHookVerificationResponse{FileBrowserHookVerification: result}, nil
}

func (h *Handler) observe(ctx context.Context, method string, route string, status int, start time.Time) {
	h.recorder.ObserveHTTP(ctx, method, route, status, time.Since(start))
}

func (h *Handler) recordAudit(ctx context.Context, decision auditDecision) {
	if decision.authorizationKind == "" {
		decision.authorizationKind = "kubeconfig"
	}
	if decision.executionKind == "" {
		decision.executionKind = "management_service_account"
	}
	if decision.identityMethod == "" {
		decision.identityMethod = "kubeconfig_context"
	}
	if decision.decision == "" {
		decision.decision = "allow"
	}
	attrs := []slog.Attr{
		slog.String("route", decision.route),
		slog.String("namespace", decision.namespace),
		slog.String("auth_mode", string(decision.mode)),
		slog.Bool("implicit_admin_elevation", decision.implicitElevation),
		slog.String("identity_method", decision.identityMethod),
		slog.String("authorization_credential", decision.authorizationKind),
		slog.String("execution_credential", decision.executionKind),
		slog.Bool("admin_allowed", decision.adminAllowed),
		slog.Bool("namespace_allowed", decision.namespaceAllowed),
		slog.String("decision", decision.decision),
	}
	if decision.denyReason != "" {
		attrs = append(attrs, slog.String("deny_reason", decision.denyReason))
	}
	if decision.principal != nil {
		attrs = append(attrs,
			slog.String("principal_id", decision.principal.ID),
			slog.String("principal_context_name", decision.principal.ContextName),
			slog.String("principal_namespace", decision.principal.Namespace),
		)
	}
	if decision.kubernetesUsername != "" {
		attrs = append(attrs, slog.String("kubernetes_username", decision.kubernetesUsername))
	}
	if decision.pvcName != "" {
		attrs = append(attrs, slog.String("pvc_name", decision.pvcName))
	}
	if decision.viewerSessionID != "" {
		attrs = append(attrs, slog.String("viewer_session_id", decision.viewerSessionID))
	}
	if decision.podSessionID != "" {
		attrs = append(attrs, slog.String("pod_session_id", decision.podSessionID))
	}
	ctx, finish := h.recorder.TraceOperation(ctx, "authorization.audit", attrs...)
	defer finish(nil)
	h.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "authorization.audit", attrs...)
}

func allowDeny(allowed bool) string {
	if allowed {
		return "allow"
	}
	return "deny"
}

func denyReason(err error, reason string) string {
	if err == nil {
		return ""
	}
	if strings.TrimSpace(reason) != "" {
		return reason
	}
	return "denied"
}

func identityMethodForOperation(op *operationContext) string {
	if op != nil && op.mode == operationModeAdmin {
		return "kubeconfig_context+self_subject_review"
	}
	return "kubeconfig_context"
}

func (h *Handler) authenticateRequest(req interface{ authorizationHeader() string }) (*authn.Principal, *apienv.Error) {
	principal, err := authn.PrincipalFromAuthorization(req.authorizationHeader())
	if err != nil {
		return nil, apienv.NewError(401, apienv.CodeUnauthorized, "Unauthorized", nil)
	}
	if h.debug.Enabled && strings.TrimSpace(h.debug.ForcedNamespace) != "" {
		principal.Namespace = strings.TrimSpace(h.debug.ForcedNamespace)
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

func (req *CreatePVCRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *ExpandPVCRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *StorageClassYAMLRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *StorageClassPolicyRequest) authorizationHeader() string {
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
	if _, ok := response.(*PVCResponse); ok {
		return http.StatusOK
	}
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
		Details(ErrorDetails{
			Code:    apiErr.Code,
			Details: apiErr.Details,
			Message: apiErr.Message,
		}).
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
	case http.StatusBadGateway, http.StatusServiceUnavailable:
		return errs.Unavailable
	case http.StatusGatewayTimeout:
		return errs.DeadlineExceeded
	default:
		return errs.Internal
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
	if session.Namespace == "" || session.PVCName == "" {
		return apienv.NewError(500, apienv.CodeInternal, "Viewer session is missing PVC identity", nil)
	}
	if session.AdminContext {
		return h.authorizeAdminSessionPVC(ctx, principal, session.Namespace, session.PVCName, viewerSessionID, "")
	}
	if err := h.authz.CanGetPVC(ctx, principal, session.Namespace, session.PVCName); err != nil {
		h.recordAudit(ctx, auditDecision{
			decision:         "deny",
			denyReason:       "ssar_denied",
			mode:             operationModeUser,
			namespace:        session.Namespace,
			namespaceAllowed: true,
			principal:        principal,
			pvcName:          session.PVCName,
			viewerSessionID:  viewerSessionID,
			route:            "/api/viewer-sessions/:id",
		})
		return apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
	}
	h.recordAudit(ctx, auditDecision{
		decision:         "allow",
		mode:             operationModeUser,
		namespace:        session.Namespace,
		namespaceAllowed: true,
		principal:        principal,
		pvcName:          session.PVCName,
		viewerSessionID:  viewerSessionID,
		route:            "/api/viewer-sessions/:id",
	})
	return nil
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
	if podSession.AdminContext {
		return h.authorizeAdminSessionPVC(ctx, principal, podSession.Namespace, podSession.PVCName, "", podSessionID)
	}
	if err := h.authz.CanGetPVC(ctx, principal, podSession.Namespace, podSession.PVCName); err != nil {
		h.recordAudit(ctx, auditDecision{
			decision:         "deny",
			denyReason:       "ssar_denied",
			mode:             operationModeUser,
			namespace:        podSession.Namespace,
			namespaceAllowed: true,
			podSessionID:     podSessionID,
			principal:        principal,
			pvcName:          podSession.PVCName,
			route:            "/api/pod-sessions/:id",
		})
		return apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
	}
	h.recordAudit(ctx, auditDecision{
		decision:         "allow",
		mode:             operationModeUser,
		namespace:        podSession.Namespace,
		namespaceAllowed: true,
		podSessionID:     podSessionID,
		principal:        principal,
		pvcName:          podSession.PVCName,
		route:            "/api/pod-sessions/:id",
	})
	return nil
}

func (h *Handler) authorizeAdminSessionPVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
	pvcName string,
	viewerSessionID string,
	podSessionID string,
) error {
	adminResult, adminErr := h.checkAdmin(ctx, principal)
	if adminErr != nil {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       false,
			decision:           "deny",
			denyReason:         denyReason(adminErr, adminResult.Reason),
			identityMethod:     "kubeconfig_context+self_subject_review",
			implicitElevation:  true,
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          namespace,
			namespaceAllowed:   false,
			podSessionID:       podSessionID,
			principal:          principal,
			pvcName:            pvcName,
			route:              sessionAuditRoute(viewerSessionID, podSessionID),
			viewerSessionID:    viewerSessionID,
		})
		return apienv.NewError(403, apienv.CodeAdminAccessDenied, "Admin access denied", nil)
	}
	namespaces, err := h.viewers.ListNamespaces(ctx)
	if err != nil {
		return err
	}
	namespaceAllowed := isAdminNamespaceAllowed(namespaces, principal.Namespace, namespace)
	if !namespaceAllowed {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       true,
			decision:           "deny",
			denyReason:         "namespace_not_allowed",
			identityMethod:     "kubeconfig_context+self_subject_review",
			implicitElevation:  true,
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          namespace,
			namespaceAllowed:   false,
			podSessionID:       podSessionID,
			principal:          principal,
			pvcName:            pvcName,
			route:              sessionAuditRoute(viewerSessionID, podSessionID),
			viewerSessionID:    viewerSessionID,
		})
		return apienv.NewError(403, apienv.CodePVCAccessDenied, "Namespace is not allowed for admin PVC access", nil)
	}
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       true,
		decision:           "allow",
		identityMethod:     "kubeconfig_context+self_subject_review",
		implicitElevation:  true,
		kubernetesUsername: adminResult.KubernetesUsername,
		mode:               operationModeAdmin,
		namespace:          namespace,
		namespaceAllowed:   true,
		podSessionID:       podSessionID,
		principal:          principal,
		pvcName:            pvcName,
		route:              sessionAuditRoute(viewerSessionID, podSessionID),
		viewerSessionID:    viewerSessionID,
	})
	return nil
}

func sessionAuditRoute(viewerSessionID string, podSessionID string) string {
	if viewerSessionID != "" {
		return "/api/viewer-sessions/:id"
	}
	if podSessionID != "" {
		return "/api/pod-sessions/:id"
	}
	return "/api/session"
}

func pathID(path string, prefix string) string {
	return strings.Trim(strings.TrimPrefix(path, prefix), "/")
}

func pvcPathParams(path string) (string, string) {
	remainder := strings.Trim(strings.TrimPrefix(path, "/api/pvcs/"), "/")
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func expandPVCPathParams(path string) (string, string) {
	remainder := strings.TrimSuffix(strings.Trim(strings.TrimPrefix(path, "/api/pvcs/"), "/"), "/expand")
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

var errRuntimeUnavailable = errors.New("viewer runtime is not configured")

type kubernetesAuthorizer struct {
	management kubernetes.Interface
	recorder   *observability.Recorder
}

func newKubernetesAuthorizer(
	management kubernetes.Interface,
	recorder *observability.Recorder,
) kubernetesAuthorizer {
	return kubernetesAuthorizer{
		management: management,
		recorder:   recorder,
	}
}

func (a kubernetesAuthorizer) CanListPVCs(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx,
		"kubernetes.authorize.list_pvcs",
		slog.String("namespace", namespace),
		slog.Bool("management_client", a.management != nil),
	)
	defer func() {
		finish(err)
	}()

	clientset, err := kubernetesClientsetForConfig(principal.ClientConfig)
	if err != nil {
		return err
	}
	if a.management != nil {
		if err := a.sameNamespace(ctx, clientset, namespace); err != nil {
			return err
		}
	}
	return a.observeKubernetes(ctx, "list", "persistentvolumeclaims", namespace, "", func(ctx context.Context) error {
		_, err := clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{Limit: 1})
		return err
	})
}

func (a kubernetesAuthorizer) CanGetPVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
	name string,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx,
		"kubernetes.authorize.get_pvc",
		slog.String("namespace", namespace),
		slog.String("pvc_name", name),
		slog.Bool("management_client", a.management != nil),
	)
	defer func() {
		finish(err)
	}()

	clientset, err := kubernetesClientsetForConfig(principal.ClientConfig)
	if err != nil {
		return err
	}
	var userPVCUID string
	err = a.observeKubernetes(ctx, "get", "persistentvolumeclaim", namespace, name, func(ctx context.Context) error {
		userPVC, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		userPVCUID = string(userPVC.UID)
		return nil
	})
	if a.management == nil {
		return err
	}
	if err != nil {
		return err
	}
	var managedPVCUID string
	err = a.observeKubernetes(ctx, "get", "persistentvolumeclaim", namespace, name, func(ctx context.Context) error {
		managedPVC, err := a.management.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		managedPVCUID = string(managedPVC.UID)
		return nil
	})
	if err != nil {
		return err
	}
	if userPVCUID != managedPVCUID {
		return errors.New("user kubeconfig and management kubeconfig resolved different PVCs")
	}
	return nil
}

func (a kubernetesAuthorizer) CanCreatePVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx,
		"kubernetes.authorize.create_pvc",
		slog.String("namespace", namespace),
		slog.Bool("management_client", a.management != nil),
	)
	defer func() {
		finish(err)
	}()

	clientset, err := kubernetesClientsetForConfig(principal.ClientConfig)
	if err != nil {
		return err
	}
	if a.management != nil {
		if err := a.sameNamespace(ctx, clientset, namespace); err != nil {
			return err
		}
	}
	return a.canUseResource(ctx, clientset, "create", "", "persistentvolumeclaims", namespace, "")
}

func (a kubernetesAuthorizer) CanDeletePVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
	name string,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx,
		"kubernetes.authorize.delete_pvc",
		slog.String("namespace", namespace),
		slog.String("pvc_name", name),
		slog.Bool("management_client", a.management != nil),
	)
	defer func() {
		finish(err)
	}()

	if err := a.CanGetPVC(ctx, principal, namespace, name); err != nil {
		return err
	}
	clientset, err := kubernetesClientsetForConfig(principal.ClientConfig)
	if err != nil {
		return err
	}
	return a.canUseResource(ctx, clientset, "delete", "", "persistentvolumeclaims", namespace, name)
}

func (a kubernetesAuthorizer) CanUpdatePVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
	name string,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx,
		"kubernetes.authorize.update_pvc",
		slog.String("namespace", namespace),
		slog.String("pvc_name", name),
		slog.Bool("management_client", a.management != nil),
	)
	defer func() {
		finish(err)
	}()

	if err := a.CanGetPVC(ctx, principal, namespace, name); err != nil {
		return err
	}
	clientset, err := kubernetesClientsetForConfig(principal.ClientConfig)
	if err != nil {
		return err
	}
	return a.canUseResource(ctx, clientset, "update", "", "persistentvolumeclaims", namespace, name)
}

func (a kubernetesAuthorizer) CanListStorageClasses(
	ctx context.Context,
	principal *authn.Principal,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx, "kubernetes.authorize.list_storageclasses")
	defer func() {
		finish(err)
	}()

	clientset, err := kubernetesClientsetForConfig(principal.ClientConfig)
	if err != nil {
		return err
	}
	return a.canUseResource(ctx, clientset, "list", "storage.k8s.io", "storageclasses", "", "")
}

func (a kubernetesAuthorizer) sameNamespace(
	ctx context.Context,
	userClient kubernetes.Interface,
	namespace string,
) error {
	var userNamespaceUID string
	err := a.observeKubernetes(ctx, "get", "namespace", namespace, "", func(ctx context.Context) error {
		userNamespace, err := userClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			return err
		}
		userNamespaceUID = string(userNamespace.UID)
		return nil
	})
	if err != nil {
		return err
	}
	var managedNamespaceUID string
	err = a.observeKubernetes(ctx, "get", "namespace", namespace, "", func(ctx context.Context) error {
		managedNamespace, err := a.management.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			return err
		}
		managedNamespaceUID = string(managedNamespace.UID)
		return nil
	})
	if err != nil {
		return err
	}
	if userNamespaceUID != managedNamespaceUID {
		return errors.New("user kubeconfig and management kubeconfig resolved different namespaces")
	}
	return nil
}

func (a kubernetesAuthorizer) observeKubernetes(
	ctx context.Context,
	operation string,
	resource string,
	namespace string,
	name string,
	call func(context.Context) error,
) (err error) {
	start := time.Now()
	attrs := []slog.Attr{
		slog.String("operation", operation),
		slog.String("resource", resource),
		slog.String("namespace", namespace),
	}
	if name != "" {
		attrs = append(attrs, slog.String("name", name))
	}
	ctx, finish := a.recorder.TraceOperation(ctx, "kubernetes."+operation, attrs...)
	defer func() {
		a.recorder.ObserveKubernetes(operation, resource, err, time.Since(start))
		finish(err)
	}()
	return call(ctx)
}

func (a kubernetesAuthorizer) canUseResource(
	ctx context.Context,
	clientset kubernetes.Interface,
	verb string,
	group string,
	resource string,
	namespace string,
	name string,
) error {
	return a.observeKubernetes(ctx, verb, resource, namespace, name, func(ctx context.Context) error {
		review, err := clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(
			ctx,
			&authorizationv1.SelfSubjectAccessReview{
				Spec: authorizationv1.SelfSubjectAccessReviewSpec{
					ResourceAttributes: &authorizationv1.ResourceAttributes{
						Group:     group,
						Verb:      verb,
						Resource:  resource,
						Namespace: namespace,
						Name:      name,
					},
				},
			},
			metav1.CreateOptions{},
		)
		if err != nil {
			return err
		}
		if !review.Status.Allowed {
			return errors.New("self subject access review denied")
		}
		return nil
	})
}
