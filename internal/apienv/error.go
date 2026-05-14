package apienv

import (
	"encoding/json"
	"errors"
	"net/http"
)

const (
	CodePVCNotFound            = "PVC_NOT_FOUND"
	CodePVCAccessDenied        = "PVC_ACCESS_DENIED"
	CodeUnsupportedAccessMode  = "UNSUPPORTED_ACCESS_MODE"
	CodePVCMountConflict       = "PVC_MOUNT_CONFLICT"
	CodePVCMountPending        = "PVC_MOUNT_PENDING"
	CodeViewerPodCreating      = "VIEWER_POD_CREATING"
	CodeViewerPodFailed        = "VIEWER_POD_FAILED"
	CodeViewerSessionNotFound  = "VIEWER_SESSION_NOT_FOUND"
	CodeViewerSessionExpired   = "VIEWER_SESSION_EXPIRED"
	CodeAuthRequestExpired     = "AUTH_REQUEST_EXPIRED"
	CodeAuthRequestUsed        = "AUTH_REQUEST_USED"
	CodeFileBrowserLoginFailed = "FILEBROWSER_LOGIN_FAILED"
	CodeHookVerifyFailed       = "HOOK_VERIFY_FAILED"
	CodeUnauthorized           = "UNAUTHORIZED"
	CodeValidationError        = "VALIDATION_ERROR"
	CodeInternal               = "INTERNAL_ERROR"
)

type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
	Status  int            `json:"-"`
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	return err.Code + ": " + err.Message
}

func NewError(status int, code string, message string, details map[string]any) *Error {
	if details == nil {
		details = map[string]any{}
	}
	return &Error{
		Code:    code,
		Message: message,
		Details: details,
		Status:  status,
	}
}

func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	if apiErr, ok := errors.AsType[*Error](err); ok {
		return apiErr
	}
	return NewError(http.StatusInternalServerError, CodeInternal, "Internal server error", nil)
}

func WriteSuccess(w http.ResponseWriter, status int, resource string, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{resource: value})
}

func WriteError(w http.ResponseWriter, err error) {
	apiErr := FromError(err)
	status := apiErr.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": apiErr})
}
