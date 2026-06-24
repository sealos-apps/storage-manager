package viewer

import (
	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
)

type AuthenticatedRequest struct {
	// Authorization carries the caller's Sealos or Kubernetes bearer token.
	Authorization string `header:"Authorization" encore:"sensitive"`
}

type StorageQuotaRequest struct {
	// Authorization carries the caller's Sealos or Kubernetes bearer token.
	Authorization string `header:"Authorization" encore:"sensitive"`
	// Namespace selects the namespace whose storage quota should be returned.
	Namespace string `query:"namespace"`
	// SealosAccountAuthorization carries the Sealos account token used for quota lookup.
	SealosAccountAuthorization string `header:"X-Sealos-Account-Authorization" encore:"sensitive"`
}

type ListPVCsRequest struct {
	// Authorization carries the caller's Sealos or Kubernetes bearer token.
	Authorization string `header:"Authorization" encore:"sensitive"`
	// Namespace filters the PVC list to one Kubernetes namespace.
	Namespace string `query:"namespace"`
}

type CreatePVCRequest struct {
	// Authorization carries the caller's Sealos or Kubernetes bearer token.
	Authorization string `header:"Authorization" encore:"sensitive"`
	// SealosAccountAuthorization carries the Sealos account token used for quota checks.
	SealosAccountAuthorization string `header:"X-Sealos-Account-Authorization" encore:"sensitive"`
	// Namespace is the Kubernetes namespace where the PVC will be created.
	Namespace string `json:"namespace"`
	// Name is the new PVC name and must be a DNS-1123 label.
	Name string `json:"name"`
	// Capacity is the Kubernetes storage quantity requested for the PVC, such as "10Gi".
	Capacity string `json:"capacity"`
	// CapacityBytes is the requested storage size in bytes.
	CapacityBytes int64 `json:"capacity_bytes"`
	// AccessModes lists Kubernetes PVC access modes such as ReadWriteOnce or ReadWriteMany.
	AccessModes []string `json:"access_modes"`
	// StorageClassName is the Kubernetes StorageClass to use for provisioning.
	StorageClassName string `json:"storage_class_name"`
}

type ExpandPVCRequest struct {
	// Authorization carries the caller's Sealos or Kubernetes bearer token.
	Authorization string `header:"Authorization" encore:"sensitive"`
	// SealosAccountAuthorization carries the Sealos account token used for quota checks.
	SealosAccountAuthorization string `header:"X-Sealos-Account-Authorization" encore:"sensitive"`
	// Capacity is the target Kubernetes storage quantity, such as "20Gi".
	Capacity string `json:"capacity"`
	// CapacityBytes is the target storage size in bytes and must be greater than the current request.
	CapacityBytes int64 `json:"capacity_bytes"`
}

type CreateViewerSessionRequest struct {
	// Authorization carries the caller's Sealos or Kubernetes bearer token.
	Authorization string `header:"Authorization" encore:"sensitive"`
	// Namespace is the Kubernetes namespace containing the PVC.
	Namespace string `json:"namespace"`
	// PVCName is the PersistentVolumeClaim to expose through File Browser.
	PVCName string `json:"pvc_name"`
}

type VerifyFileBrowserHookRequest struct {
	// Authorization carries the internal hook bearer token configured for File Browser.
	Authorization string `header:"Authorization" encore:"sensitive"`
	// PodSessionID identifies the backing viewer pod session.
	PodSessionID string `json:"pod_session_id"`
	// ViewerPodName is the Kubernetes pod name making the hook request.
	ViewerPodName string `json:"viewer_pod_name"`
	// Username is the File Browser username being authenticated.
	Username string `json:"username"`
	// AuthRequestID identifies the one-time login request issued by storage-manager.
	AuthRequestID string `json:"auth_request_id"`
	// PasswordHash is the File Browser password hash presented for verification.
	PasswordHash string `json:"password_hash"`
}

type PVCList struct {
	// Items contains PVC summaries visible to the caller.
	Items []domain.PVC `json:"items"`
}

type ListPVCsResponse struct {
	// PVCList wraps the PVC collection returned by list operations.
	PVCList PVCList `json:"pvc_list"`
}

type NamespaceList struct {
	// Items contains namespaces available to the caller.
	Items []domain.Namespace `json:"items"`
}

type ListNamespacesResponse struct {
	// NamespaceList wraps the namespace collection returned to administrators.
	NamespaceList NamespaceList `json:"namespace_list"`
}

type ViewerContext struct {
	// ContextName is the Kubernetes context name resolved from the caller credentials.
	ContextName string `json:"context_name"`
	// Namespace is the default namespace resolved for user-scoped operations.
	Namespace string `json:"namespace"`
}

type ContextResponse struct {
	// Context contains the Kubernetes context metadata for the caller.
	Context ViewerContext `json:"context"`
}

type StorageQuota struct {
	// AvailableBytes is the remaining storage quota in bytes.
	AvailableBytes int64 `json:"available_bytes"`
	// AvailableQuantity is the remaining storage quota as a Kubernetes quantity string.
	AvailableQuantity string `json:"available_quantity"`
	// LimitBytes is the total storage quota limit in bytes.
	LimitBytes int64 `json:"limit_bytes"`
	// LimitQuantity is the total storage quota limit as a Kubernetes quantity string.
	LimitQuantity string `json:"limit_quantity"`
	// UsedBytes is the storage currently consumed in bytes.
	UsedBytes int64 `json:"used_bytes"`
	// UsedQuantity is the storage currently consumed as a Kubernetes quantity string.
	UsedQuantity string `json:"used_quantity"`
}

type StorageQuotaResponse struct {
	// StorageQuota contains namespace storage usage and limits.
	StorageQuota StorageQuota `json:"storage_quota"`
}

type PVCResponse struct {
	// PVC contains the created, updated, or deleted PersistentVolumeClaim summary.
	PVC *domain.PVC `json:"pvc"`
}

type PVCYAMLResponse struct {
	// PVCYAML contains editable YAML for a PersistentVolumeClaim.
	PVCYAML *session.PVCYAML `json:"pvc_yaml"`
}

type PVCYAMLRequest struct {
	// Authorization carries the caller's Sealos or Kubernetes bearer token.
	Authorization string `header:"Authorization" encore:"sensitive"`
	// SealosAccountAuthorization carries the Sealos account token used for quota checks.
	SealosAccountAuthorization string `header:"X-Sealos-Account-Authorization" encore:"sensitive"`
	// YAML is a v1 PersistentVolumeClaim manifest.
	YAML string `json:"yaml"`
}

type PVCDescribeResponse struct {
	// PVCDescribe contains kubectl-style PVC diagnostic text.
	PVCDescribe *session.PVCDescribe `json:"pvc_describe"`
}

type StorageClassList struct {
	// Items contains Kubernetes StorageClass summaries.
	Items []domain.StorageClass `json:"items"`
}

type ListStorageClassesResponse struct {
	// StorageClassList wraps the StorageClass collection.
	StorageClassList StorageClassList `json:"storage_class_list"`
}

type AdminCapabilitySet struct {
	// CanManagePVCs reports whether the caller can manage PVCs across namespaces.
	CanManagePVCs bool `json:"can_manage_pvcs"`
	// CanManageStorageClasses reports whether the caller can create, update, and delete StorageClasses.
	CanManageStorageClasses bool `json:"can_manage_storage_classes"`
	// FileManagementEnabled reports whether File Browser viewer sessions are enabled.
	FileManagementEnabled bool `json:"file_management_enabled"`
	// PVCCreationEnabled reports whether PVC creation is enabled by configuration.
	PVCCreationEnabled bool `json:"pvc_creation_enabled"`
	// UserNamespace is the namespace resolved from the caller credentials.
	UserNamespace string `json:"user_namespace"`
}

type AdminCapabilitiesResponse struct {
	// AdminCapabilities contains feature and authorization flags for the caller.
	AdminCapabilities AdminCapabilitySet `json:"admin_capabilities"`
}

type StorageClassResponse struct {
	// StorageClass contains the created, updated, deleted, or fetched StorageClass summary.
	StorageClass *domain.StorageClass `json:"storage_class"`
}

type StorageClassYAMLResponse struct {
	// StorageClassYAML contains editable YAML for a StorageClass.
	StorageClassYAML *session.StorageClassYAML `json:"storage_class_yaml"`
}

type StorageClassYAMLRequest struct {
	// Authorization carries the caller's Sealos or Kubernetes bearer token.
	Authorization string `header:"Authorization" encore:"sensitive"`
	// YAML is a storage.k8s.io/v1 StorageClass manifest.
	YAML string `json:"yaml"`
}

type StorageClassMetadataRequest struct {
	// Authorization carries the caller's Sealos or Kubernetes bearer token.
	Authorization string `header:"Authorization" encore:"sensitive"`
	// AvailableToUsers mirrors the storage-manager annotation for future user-facing policy.
	AvailableToUsers bool `json:"available_to_users"`
	// DisplayNames maps locale codes to UI display names.
	DisplayNames map[string]string `json:"display_names"`
}

type StorageClassDescribeResponse struct {
	// StorageClassDescribe contains kubectl-style StorageClass diagnostic text.
	StorageClassDescribe *session.StorageClassDescribe `json:"storage_class_describe"`
}

type ViewerSessionResponse struct {
	// ViewerSession contains the user-scoped File Browser session state.
	ViewerSession *domain.ViewerSession `json:"viewer_session"`
}

type ViewerTokenResponse struct {
	// CacheControl instructs clients and intermediaries not to store the token response.
	CacheControl string `header:"Cache-Control"`
	// Pragma provides legacy no-cache behavior for token responses.
	Pragma string `header:"Pragma"`
	// ViewerToken contains the short-lived File Browser login token.
	ViewerToken *domain.ViewerToken `json:"viewer_token"`
}

type HeartbeatResponse struct {
	// Heartbeat contains refreshed session activity and expiry information.
	Heartbeat *domain.Heartbeat `json:"heartbeat"`
}

type PodSessionResponse struct {
	// PodSession contains the backing Kubernetes pod session state.
	PodSession *domain.PodSession `json:"pod_session"`
}

type FileBrowserHookVerificationResponse struct {
	// FileBrowserHookVerification tells File Browser whether to allow the login attempt.
	FileBrowserHookVerification domain.FileBrowserHookVerification `json:"filebrowser_hook_verification"`
}

type ErrorDetails struct {
	// Code is the storage-manager application error code.
	Code apienv.Code `json:"code"`
	// Details contains structured, non-sensitive context for the failure.
	Details map[string]any `json:"details,omitempty"`
	// Message is the human-readable error message.
	Message string `json:"message,omitempty"`
}

func (ErrorDetails) ErrDetails() {}
