package viewer

import (
	"context"
	"net/http"
)

var defaultHandler *Handler

//encore:api public method=GET path=/pvcs
func ListPVCs(ctx context.Context, req *ListPVCsRequest) (*ListPVCsResponse, error) {
	return runtimeHandler().ListPVCsData(ctx, req)
}

//encore:api public method=GET path=/context
func GetContext(ctx context.Context, req *AuthenticatedRequest) (*ContextResponse, error) {
	return runtimeHandler().GetContextData(ctx, req)
}

//encore:api public method=POST path=/pvcs
func CreatePVC(ctx context.Context, req *CreatePVCRequest) (*PVCResponse, error) {
	return runtimeHandler().CreatePVCData(ctx, req)
}

//encore:api public method=DELETE path=/pvcs/:namespace/:name
func DeletePVC(
	ctx context.Context,
	namespace string,
	name string,
	req *AuthenticatedRequest,
) (*PVCResponse, error) {
	return runtimeHandler().DeletePVCData(ctx, namespace, name, req)
}

//encore:api public method=POST path=/pvcs/:namespace/:name/expand
func ExpandPVC(
	ctx context.Context,
	namespace string,
	name string,
	req *ExpandPVCRequest,
) (*PVCResponse, error) {
	return runtimeHandler().ExpandPVCData(ctx, namespace, name, req)
}

//encore:api public method=GET path=/storage-classes
func ListStorageClasses(ctx context.Context, req *AuthenticatedRequest) (*ListStorageClassesResponse, error) {
	return runtimeHandler().ListStorageClassesData(ctx, req)
}

//encore:api public method=GET path=/admin/capabilities
func AdminCapabilities(ctx context.Context, req *AuthenticatedRequest) (*AdminCapabilitiesResponse, error) {
	return runtimeHandler().AdminCapabilitiesData(ctx, req)
}

//encore:api public method=GET path=/admin/namespaces
func AdminListNamespaces(ctx context.Context, req *AuthenticatedRequest) (*ListNamespacesResponse, error) {
	return runtimeHandler().AdminListNamespacesData(ctx, req)
}

//encore:api public method=GET path=/admin/storage-classes
func AdminListStorageClasses(ctx context.Context, req *AuthenticatedRequest) (*ListStorageClassesResponse, error) {
	return runtimeHandler().AdminListStorageClassesData(ctx, req)
}

//encore:api public method=GET path=/admin/storage-classes/:name/yaml
func AdminGetStorageClassYAML(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassYAMLResponse, error) {
	return runtimeHandler().AdminGetStorageClassYAMLData(ctx, name, req)
}

//encore:api public method=POST path=/admin/storage-classes
func AdminCreateStorageClass(ctx context.Context, req *StorageClassYAMLRequest) (*StorageClassResponse, error) {
	return runtimeHandler().AdminCreateStorageClassData(ctx, req)
}

//encore:api public method=PUT path=/admin/storage-classes/:name
func AdminUpdateStorageClass(
	ctx context.Context,
	name string,
	req *StorageClassYAMLRequest,
) (*StorageClassResponse, error) {
	return runtimeHandler().AdminUpdateStorageClassData(ctx, name, req)
}

//encore:api public method=PUT path=/admin/storage-classes/:name/policy
func AdminUpdateStorageClassPolicy(
	ctx context.Context,
	name string,
	req *StorageClassPolicyRequest,
) (*StorageClassResponse, error) {
	return runtimeHandler().AdminUpdateStorageClassPolicyData(ctx, name, req)
}

//encore:api public method=DELETE path=/admin/storage-classes/:name
func AdminDeleteStorageClass(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassResponse, error) {
	return runtimeHandler().AdminDeleteStorageClassData(ctx, name, req)
}

//encore:api public method=GET path=/admin/storage-classes/:name/describe
func AdminDescribeStorageClass(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassDescribeResponse, error) {
	return runtimeHandler().AdminDescribeStorageClassData(ctx, name, req)
}

//encore:api public method=POST path=/viewer-sessions
func CreateViewerSession(
	ctx context.Context,
	req *CreateViewerSessionRequest,
) (*ViewerSessionResponse, error) {
	return runtimeHandler().CreateViewerSessionData(ctx, req)
}

//encore:api public method=GET path=/viewer-sessions/:viewerSessionID
func GetViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, error) {
	return runtimeHandler().GetViewerSessionData(ctx, viewerSessionID, req)
}

//encore:api public method=POST path=/viewer-sessions/:viewerSessionID/token
func IssueViewerToken(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerTokenResponse, error) {
	return runtimeHandler().IssueTokenData(ctx, viewerSessionID, req)
}

//encore:api public method=POST path=/viewer-sessions/:viewerSessionID/heartbeat
func HeartbeatViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*HeartbeatResponse, error) {
	return runtimeHandler().HeartbeatData(ctx, viewerSessionID, req)
}

//encore:api public method=DELETE path=/viewer-sessions/:viewerSessionID
func CloseViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, error) {
	return runtimeHandler().CloseViewerSessionData(ctx, viewerSessionID, req)
}

//encore:api public method=DELETE path=/pod-sessions/:podSessionID
func ClosePodSession(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, error) {
	return runtimeHandler().ClosePodSessionData(ctx, podSessionID, req)
}

//encore:api public method=GET path=/pod-sessions/:podSessionID
func GetPodSession(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, error) {
	return runtimeHandler().GetPodSessionData(ctx, podSessionID, req)
}

//encore:api public method=POST path=/internal/filebrowser-hook/verify
func VerifyFileBrowserHook(
	ctx context.Context,
	req *VerifyFileBrowserHookRequest,
) (*FileBrowserHookVerificationResponse, error) {
	return runtimeHandler().VerifyFileBrowserHookData(ctx, req)
}

//encore:api public raw method=GET path=/metrics
func Metrics(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().Metrics(w, req)
}
