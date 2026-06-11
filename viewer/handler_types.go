package viewer

import (
	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
)

type AuthenticatedRequest struct {
	Authorization string `header:"Authorization" encore:"sensitive"`
}

type StorageQuotaRequest struct {
	Authorization              string `header:"Authorization" encore:"sensitive"`
	Namespace                  string `query:"namespace"`
	SealosAccountAuthorization string `header:"X-Sealos-Account-Authorization" encore:"sensitive"`
}

type ListPVCsRequest struct {
	Authorization string `header:"Authorization" encore:"sensitive"`
	Namespace     string `query:"namespace"`
}

type CreatePVCRequest struct {
	Authorization              string   `header:"Authorization" encore:"sensitive"`
	SealosAccountAuthorization string   `header:"X-Sealos-Account-Authorization" encore:"sensitive"`
	Namespace                  string   `json:"namespace"`
	Name                       string   `json:"name"`
	Capacity                   string   `json:"capacity"`
	CapacityBytes              int64    `json:"capacity_bytes"`
	AccessModes                []string `json:"access_modes"`
	StorageClassName           string   `json:"storage_class_name"`
}

type ExpandPVCRequest struct {
	Authorization              string `header:"Authorization" encore:"sensitive"`
	SealosAccountAuthorization string `header:"X-Sealos-Account-Authorization" encore:"sensitive"`
	Capacity                   string `json:"capacity"`
	CapacityBytes              int64  `json:"capacity_bytes"`
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

type StorageQuota struct {
	AvailableBytes    int64  `json:"available_bytes"`
	AvailableQuantity string `json:"available_quantity"`
	LimitBytes        int64  `json:"limit_bytes"`
	LimitQuantity     string `json:"limit_quantity"`
	UsedBytes         int64  `json:"used_bytes"`
	UsedQuantity      string `json:"used_quantity"`
}

type StorageQuotaResponse struct {
	StorageQuota StorageQuota `json:"storage_quota"`
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
	CanManagePVCs           bool   `json:"can_manage_pvcs"`
	CanManageStorageClasses bool   `json:"can_manage_storage_classes"`
	FileManagementEnabled   bool   `json:"file_management_enabled"`
	PVCCreationEnabled      bool   `json:"pvc_creation_enabled"`
	UserNamespace           string `json:"user_namespace"`
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
