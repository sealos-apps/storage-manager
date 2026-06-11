package apienv

import (
	"encoding/json"
	"errors"
	"net/http"
)

const (
	CodePVCAlreadyExists            Code = "PVC_ALREADY_EXISTS"
	CodePVCNotFound                 Code = "PVC_NOT_FOUND"
	CodePVCAccessDenied             Code = "PVC_ACCESS_DENIED"
	CodePVCInUse                    Code = "PVC_IN_USE"
	CodePVCCreateForbidden          Code = "PVC_CREATE_FORBIDDEN"
	CodePVCDeleteForbidden          Code = "PVC_DELETE_FORBIDDEN"
	CodePVCExpandForbidden          Code = "PVC_EXPAND_FORBIDDEN"
	CodePVCQuotaExceeded            Code = "PVC_QUOTA_EXCEEDED"
	CodePVCQuotaUnavailable         Code = "PVC_QUOTA_UNAVAILABLE"
	CodePVCExpandUnsupported        Code = "PVC_EXPAND_UNSUPPORTED"
	CodePVCExpandNotIncreased       Code = "PVC_EXPAND_NOT_INCREASED"
	CodeStorageClassNotFound        Code = "STORAGE_CLASS_NOT_FOUND"
	CodeStorageClassYAMLInvalid     Code = "STORAGE_CLASS_YAML_INVALID"
	CodeStorageClassConflict        Code = "STORAGE_CLASS_CONFLICT"
	CodeStorageClassDeleteForbidden Code = "STORAGE_CLASS_DELETE_FORBIDDEN"
	CodeStorageClassInUse           Code = "STORAGE_CLASS_IN_USE"
	CodeAdminAccessDenied           Code = "ADMIN_ACCESS_DENIED"
	CodeUnsupportedAccessMode       Code = "UNSUPPORTED_ACCESS_MODE"
	CodePVCMountConflict            Code = "PVC_MOUNT_CONFLICT"
	CodePVCMountPending             Code = "PVC_MOUNT_PENDING"
	CodeViewerPodCreating           Code = "VIEWER_POD_CREATING"
	CodeViewerPodFailed             Code = "VIEWER_POD_FAILED"
	CodePodSessionNotFound          Code = "POD_SESSION_NOT_FOUND"
	CodeViewerSessionNotFound       Code = "VIEWER_SESSION_NOT_FOUND"
	CodeViewerSessionExpired        Code = "VIEWER_SESSION_EXPIRED"
	CodeAuthRequestExpired          Code = "AUTH_REQUEST_EXPIRED"
	CodeAuthRequestUsed             Code = "AUTH_REQUEST_USED"
	CodeFileBrowserLoginFailed      Code = "FILEBROWSER_LOGIN_FAILED"
	CodeFileManagementDisabled      Code = "FILE_MANAGEMENT_DISABLED"
	CodeHookVerifyFailed            Code = "HOOK_VERIFY_FAILED"
	CodeUnauthorized                Code = "UNAUTHORIZED"
	CodeValidationError             Code = "VALIDATION_ERROR"
	CodeInternal                    Code = "INTERNAL_ERROR"
)

type Code string

var AllCodes = [...]Code{
	CodePVCAlreadyExists,
	CodePVCNotFound,
	CodePVCAccessDenied,
	CodePVCInUse,
	CodePVCCreateForbidden,
	CodePVCDeleteForbidden,
	CodePVCExpandForbidden,
	CodePVCQuotaExceeded,
	CodePVCQuotaUnavailable,
	CodePVCExpandUnsupported,
	CodePVCExpandNotIncreased,
	CodeStorageClassNotFound,
	CodeStorageClassYAMLInvalid,
	CodeStorageClassConflict,
	CodeStorageClassDeleteForbidden,
	CodeStorageClassInUse,
	CodeAdminAccessDenied,
	CodeUnsupportedAccessMode,
	CodePVCMountConflict,
	CodePVCMountPending,
	CodeViewerPodCreating,
	CodeViewerPodFailed,
	CodePodSessionNotFound,
	CodeViewerSessionNotFound,
	CodeViewerSessionExpired,
	CodeAuthRequestExpired,
	CodeAuthRequestUsed,
	CodeFileBrowserLoginFailed,
	CodeFileManagementDisabled,
	CodeHookVerifyFailed,
	CodeUnauthorized,
	CodeValidationError,
	CodeInternal,
}

func IsCode(code Code) bool {
	for _, known := range AllCodes {
		if known == code {
			return true
		}
	}
	return false
}

type Error struct {
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
	Status  int            `json:"-"`
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	return string(err.Code) + ": " + err.Message
}

func NewError(status int, code Code, message string, details map[string]any) *Error {
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
