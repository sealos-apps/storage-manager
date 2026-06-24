package viewer

import (
	"encoding/json"
	"net/http"
	"strings"

	"encore.dev/beta/errs"
	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
)

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

func (h *Handler) GetStorageQuota(w http.ResponseWriter, req *http.Request) {
	response, err := h.getStorageQuota(req.Context(), &StorageQuotaRequest{
		Authorization:              req.Header.Get("Authorization"),
		Namespace:                  req.URL.Query().Get("namespace"),
		SealosAccountAuthorization: req.Header.Get("X-Sealos-Account-Authorization"),
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

func (h *Handler) CreatePVC(w http.ResponseWriter, req *http.Request) {
	var body CreatePVCRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeHTTPResponse(w, nil, apienv.NewError(400, apienv.CodeValidationError, "Invalid JSON request", nil))
		return
	}
	body.Authorization = req.Header.Get("Authorization")
	body.SealosAccountAuthorization = req.Header.Get("X-Sealos-Account-Authorization")
	response, err := h.createPVC(req.Context(), &body)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) DeletePVC(w http.ResponseWriter, req *http.Request) {
	namespace, name := pvcPathParams(req.URL.Path)
	response, err := h.deletePVC(req.Context(), namespace, name, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) GetPVCYAML(w http.ResponseWriter, req *http.Request) {
	namespace, name := pvcYAMLPathParams(req.URL.Path)
	response, err := h.getPVCYAML(req.Context(), namespace, name, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) UpdatePVC(w http.ResponseWriter, req *http.Request) {
	var body PVCYAMLRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeHTTPResponse(w, nil, apienv.NewError(400, apienv.CodeValidationError, "Invalid JSON request", nil))
		return
	}
	body.Authorization = req.Header.Get("Authorization")
	body.SealosAccountAuthorization = req.Header.Get("X-Sealos-Account-Authorization")
	namespace, name := pvcPathParams(req.URL.Path)
	response, err := h.updatePVC(req.Context(), namespace, name, &body)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) DescribePVC(w http.ResponseWriter, req *http.Request) {
	namespace, name := pvcDescribePathParams(req.URL.Path)
	response, err := h.describePVC(req.Context(), namespace, name, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) ExpandPVC(w http.ResponseWriter, req *http.Request) {
	var body ExpandPVCRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeHTTPResponse(w, nil, apienv.NewError(400, apienv.CodeValidationError, "Invalid JSON request", nil))
		return
	}
	body.Authorization = req.Header.Get("Authorization")
	body.SealosAccountAuthorization = req.Header.Get("X-Sealos-Account-Authorization")
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
	name := strings.TrimSuffix(pathID(req.URL.Path, "/admin/storage-classes/"), "/yaml")
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
	name := pathID(req.URL.Path, "/admin/storage-classes/")
	response, err := h.adminUpdateStorageClass(req.Context(), name, &body)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminUpdateStorageClassMetadata(w http.ResponseWriter, req *http.Request) {
	var body StorageClassMetadataRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeHTTPResponse(w, nil, apienv.NewError(400, apienv.CodeValidationError, "Invalid JSON request", nil))
		return
	}
	body.Authorization = req.Header.Get("Authorization")
	name := strings.TrimSuffix(pathID(req.URL.Path, "/admin/storage-classes/"), "/metadata")
	response, err := h.adminUpdateStorageClassMetadata(req.Context(), name, &body)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminDeleteStorageClass(w http.ResponseWriter, req *http.Request) {
	name := pathID(req.URL.Path, "/admin/storage-classes/")
	response, err := h.adminDeleteStorageClass(req.Context(), name, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) AdminDescribeStorageClass(w http.ResponseWriter, req *http.Request) {
	name := strings.TrimSuffix(pathID(req.URL.Path, "/admin/storage-classes/"), "/describe")
	response, err := h.adminDescribeStorageClass(req.Context(), name, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) GetViewerSession(w http.ResponseWriter, req *http.Request) {
	response, err := h.getViewerSession(
		req.Context(),
		pathID(req.URL.Path, "/viewer-sessions/"),
		authenticatedRequest(req),
	)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) IssueToken(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSuffix(pathID(req.URL.Path, "/viewer-sessions/"), "/token")
	response, err := h.issueToken(req.Context(), id, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) Heartbeat(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSuffix(pathID(req.URL.Path, "/viewer-sessions/"), "/heartbeat")
	response, err := h.heartbeat(req.Context(), id, authenticatedRequest(req))
	writeHTTPResponse(w, response, err)
}

func (h *Handler) CloseViewerSession(w http.ResponseWriter, req *http.Request) {
	response, err := h.closeViewerSession(
		req.Context(),
		pathID(req.URL.Path, "/viewer-sessions/"),
		authenticatedRequest(req),
	)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) ClosePodSession(w http.ResponseWriter, req *http.Request) {
	response, err := h.closePodSession(
		req.Context(),
		pathID(req.URL.Path, "/pod-sessions/"),
		authenticatedRequest(req),
	)
	writeHTTPResponse(w, response, err)
}

func (h *Handler) GetPodSession(w http.ResponseWriter, req *http.Request) {
	response, err := h.getPodSession(
		req.Context(),
		pathID(req.URL.Path, "/pod-sessions/"),
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
