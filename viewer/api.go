package viewer

import (
	"context"
	"net/http"
)

var defaultHandler *Handler

// List PVCs
// Returns PersistentVolumeClaims the caller can access. Regular users see their
// current namespace. Admin callers can pass namespace to inspect another
// namespace.
//
//encore:api public method=GET path=/pvcs
func ListPVCs(ctx context.Context, req *ListPVCsRequest) (*ListPVCsResponse, error) {
	return runtimeHandler().ListPVCsData(ctx, req)
}

// Get Context
// Returns the Kubernetes context resolved from the caller credentials. The
// namespace in this response is the default namespace for user-scoped PVC and
// viewer operations.
//
//encore:api public method=GET path=/context
func GetContext(ctx context.Context, req *AuthenticatedRequest) (*ContextResponse, error) {
	return runtimeHandler().GetContextData(ctx, req)
}

// Get Storage Quota
// Returns used, available, and limit storage for a namespace. Values are
// returned as bytes and Kubernetes quantity strings. Admin callers can pass
// namespace to inspect another namespace.
//
//encore:api public method=GET path=/storage-quota
func GetStorageQuota(ctx context.Context, req *StorageQuotaRequest) (*StorageQuotaResponse, error) {
	return runtimeHandler().GetStorageQuotaData(ctx, req)
}

// Create PVC
// Creates a PersistentVolumeClaim in the requested namespace. The name must be
// a DNS-1123 label. Capacity must be sent as both a Kubernetes quantity and
// bytes. Quota checks use X-Sealos-Account-Authorization when quota is enabled.
//
//encore:api public method=POST path=/pvcs
func CreatePVC(ctx context.Context, req *CreatePVCRequest) (*PVCResponse, error) {
	return runtimeHandler().CreatePVCData(ctx, req)
}

// Delete PVC
// Deletes a PersistentVolumeClaim by namespace and name. The PVC must be
// visible to the caller. Deletion is blocked while active pods still mount the
// PVC.
//
//encore:api public method=DELETE path=/pvcs/:namespace/:name
func DeletePVC(
	ctx context.Context,
	namespace string,
	name string,
	req *AuthenticatedRequest,
) (*PVCResponse, error) {
	return runtimeHandler().DeletePVCData(ctx, namespace, name, req)
}

// Expand PVC
// Increases a PersistentVolumeClaim storage request. The new capacity must be
// larger than the current value. The StorageClass must allow volume expansion.
// Quota checks use X-Sealos-Account-Authorization when quota is enabled.
//
//encore:api public method=POST path=/pvcs/:namespace/:name/expand
func ExpandPVC(
	ctx context.Context,
	namespace string,
	name string,
	req *ExpandPVCRequest,
) (*PVCResponse, error) {
	return runtimeHandler().ExpandPVCData(ctx, namespace, name, req)
}

// List Storage Classes
// Returns StorageClasses available for PVC creation. Each item includes the
// provisioner, default flag, expansion support, binding mode, and reclaim
// policy.
//
//encore:api public method=GET path=/storage-classes
func ListStorageClasses(ctx context.Context, req *AuthenticatedRequest) (*ListStorageClassesResponse, error) {
	return runtimeHandler().ListStorageClassesData(ctx, req)
}

// Get Admin Capabilities
// Returns the caller's management permissions and enabled features. The
// response covers cross-namespace PVC access, StorageClass administration, PVC
// creation, and File Browser session availability.
//
//encore:api public method=GET path=/admin/capabilities
func AdminCapabilities(ctx context.Context, req *AuthenticatedRequest) (*AdminCapabilitiesResponse, error) {
	return runtimeHandler().AdminCapabilitiesData(ctx, req)
}

// List Admin Namespaces
// Returns Kubernetes namespaces available to an admin caller. The current
// context namespace is marked in the response.
//
//encore:api public method=GET path=/admin/namespaces
func AdminListNamespaces(ctx context.Context, req *AuthenticatedRequest) (*ListNamespacesResponse, error) {
	return runtimeHandler().AdminListNamespacesData(ctx, req)
}

// List Admin Storage Classes
// Returns all cluster StorageClasses for an admin caller. Each item includes
// whether storage-manager owns it, whether deletion is blocked, and how many
// PVCs use it.
//
//encore:api public method=GET path=/admin/storage-classes
func AdminListStorageClasses(ctx context.Context, req *AuthenticatedRequest) (*ListStorageClassesResponse, error) {
	return runtimeHandler().AdminListStorageClassesData(ctx, req)
}

// Get Storage Class YAML
// Returns editable YAML for a StorageClass. Server-managed Kubernetes fields
// are removed so the YAML can be edited and sent to Update Storage Class.
//
//encore:api public method=GET path=/admin/storage-classes/:name/yaml
func AdminGetStorageClassYAML(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassYAMLResponse, error) {
	return runtimeHandler().AdminGetStorageClassYAMLData(ctx, name, req)
}

// Create Storage Class
// Creates a StorageClass from YAML. The manifest must be storage.k8s.io/v1,
// kind StorageClass, with metadata.name set and metadata.namespace empty.
// Created classes are labeled as managed by storage-manager.
//
//encore:api public method=POST path=/admin/storage-classes
func AdminCreateStorageClass(ctx context.Context, req *StorageClassYAMLRequest) (*StorageClassResponse, error) {
	return runtimeHandler().AdminCreateStorageClassData(ctx, req)
}

// Update Storage Class
// Updates a StorageClass from YAML. The manifest name must match the path name.
// On resource-version conflicts, reload the YAML before retrying.
//
//encore:api public method=PUT path=/admin/storage-classes/:name
func AdminUpdateStorageClass(
	ctx context.Context,
	name string,
	req *StorageClassYAMLRequest,
) (*StorageClassResponse, error) {
	return runtimeHandler().AdminUpdateStorageClassData(ctx, name, req)
}

// Delete Storage Class
// Deletes a StorageClass managed by storage-manager. Deletion is blocked for
// unmanaged classes and for classes still referenced by PVCs.
//
//encore:api public method=DELETE path=/admin/storage-classes/:name
func AdminDeleteStorageClass(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassResponse, error) {
	return runtimeHandler().AdminDeleteStorageClassData(ctx, name, req)
}

// Describe Storage Class
// Returns a kubectl-style text description for a StorageClass. The description
// includes provisioner, parameters, reclaim policy, binding mode, and related
// event-style details when available.
//
//encore:api public method=GET path=/admin/storage-classes/:name/describe
func AdminDescribeStorageClass(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassDescribeResponse, error) {
	return runtimeHandler().AdminDescribeStorageClassData(ctx, name, req)
}

// Create Viewer Session
// Opens File Browser access for a PVC. The endpoint validates PVC access,
// creates or reuses the backing viewer pod, and returns the user session state.
//
//encore:api public method=POST path=/viewer-sessions
func CreateViewerSession(
	ctx context.Context,
	req *CreateViewerSessionRequest,
) (*ViewerSessionResponse, error) {
	return runtimeHandler().CreateViewerSessionData(ctx, req)
}

// Get Viewer Session
// Returns the current viewer session state. The response includes viewer
// status, pod status, viewer URL, token readiness, and expiration time.
//
//encore:api public method=GET path=/viewer-sessions/:viewerSessionID
func GetViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, error) {
	return runtimeHandler().GetViewerSessionData(ctx, viewerSessionID, req)
}

// Issue Viewer Token
// Issues a short-lived File Browser login token for a ready viewer session. The
// response sets no-cache headers because the token grants direct file access.
//
//encore:api public method=POST path=/viewer-sessions/:viewerSessionID/token
func IssueViewerToken(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerTokenResponse, error) {
	return runtimeHandler().IssueTokenData(ctx, viewerSessionID, req)
}

// Heartbeat Viewer Session
// Refreshes activity for an open viewer session. The response returns server
// time, current status, and the updated expiration time.
//
//encore:api public method=POST path=/viewer-sessions/:viewerSessionID/heartbeat
func HeartbeatViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*HeartbeatResponse, error) {
	return runtimeHandler().HeartbeatData(ctx, viewerSessionID, req)
}

// Close Viewer Session
// Closes a user viewer session. The backing pod session remains available until
// cleanup removes it or no active viewer sessions depend on it.
//
//encore:api public method=DELETE path=/viewer-sessions/:viewerSessionID
func CloseViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, error) {
	return runtimeHandler().CloseViewerSessionData(ctx, viewerSessionID, req)
}

// Close Pod Session
// Terminates a backing viewer pod session. This closes the Kubernetes pod and
// service used by File Browser and returns the final pod session state.
//
//encore:api public method=DELETE path=/pod-sessions/:podSessionID
func ClosePodSession(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, error) {
	return runtimeHandler().ClosePodSessionData(ctx, podSessionID, req)
}

// Get Pod Session
// Returns a backing viewer pod session. The response includes pod, service,
// scheduling, lifecycle, and expiration details for debugging viewer
// availability.
//
//encore:api public method=GET path=/pod-sessions/:podSessionID
func GetPodSession(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, error) {
	return runtimeHandler().GetPodSessionData(ctx, podSessionID, req)
}

// Verify File Browser Hook
// Validates an internal File Browser auth hook request. The endpoint checks the
// hook token, one-time auth request, password hash, pod session, and viewer pod
// identity before File Browser allows login.
//
//encore:api public method=POST path=/internal/filebrowser-hook/verify
func VerifyFileBrowserHook(
	ctx context.Context,
	req *VerifyFileBrowserHookRequest,
) (*FileBrowserHookVerificationResponse, error) {
	return runtimeHandler().VerifyFileBrowserHookData(ctx, req)
}

// Get Metrics
// Returns Prometheus text metrics for local scraping. This raw endpoint is
// separate from the typed business API.
//
//encore:api public raw method=GET path=/metrics
func Metrics(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().Metrics(w, req)
}
