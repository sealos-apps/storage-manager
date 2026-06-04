package viewer

import (
	"context"
	"net/http"

	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
)

var defaultHandler *Handler

//encore:api public method=GET path=/api/pvcs
func ListPVCs(ctx context.Context, req *ListPVCsRequest) (*ListPVCsResponse, error) {
	return runtimeHandler().ListPVCsData(ctx, req)
}

//encore:api public method=GET path=/api/context
func GetContext(ctx context.Context, req *AuthenticatedRequest) (*ContextResponse, error) {
	return runtimeHandler().GetContextData(ctx, req)
}

//encore:api public method=POST path=/api/pvcs
func CreatePVC(ctx context.Context, req *CreatePVCRequest) (*PVCResponse, error) {
	return runtimeHandler().CreatePVCData(ctx, req)
}

//encore:api public method=DELETE path=/api/pvcs/:namespace/:name
func DeletePVC(
	ctx context.Context,
	namespace string,
	name string,
	req *AuthenticatedRequest,
) (*PVCResponse, error) {
	return runtimeHandler().DeletePVCData(ctx, namespace, name, req)
}

//encore:api public method=POST path=/api/pvcs/:namespace/:name/expand
func ExpandPVC(
	ctx context.Context,
	namespace string,
	name string,
	req *ExpandPVCRequest,
) (*PVCResponse, error) {
	return runtimeHandler().ExpandPVCData(ctx, namespace, name, req)
}

//encore:api public method=GET path=/api/storage-classes
func ListStorageClasses(ctx context.Context, req *AuthenticatedRequest) (*ListStorageClassesResponse, error) {
	return runtimeHandler().ListStorageClassesData(ctx, req)
}

//encore:api public method=POST path=/api/viewer-sessions
func CreateViewerSession(
	ctx context.Context,
	req *CreateViewerSessionRequest,
) (*ViewerSessionResponse, error) {
	return runtimeHandler().CreateViewerSessionData(ctx, req)
}

//encore:api public method=GET path=/api/viewer-sessions/:viewerSessionID
func GetViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, error) {
	return runtimeHandler().GetViewerSessionData(ctx, viewerSessionID, req)
}

//encore:api public method=POST path=/api/viewer-sessions/:viewerSessionID/token
func IssueViewerToken(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerTokenResponse, error) {
	return runtimeHandler().IssueTokenData(ctx, viewerSessionID, req)
}

//encore:api public method=POST path=/api/viewer-sessions/:viewerSessionID/heartbeat
func HeartbeatViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*HeartbeatResponse, error) {
	return runtimeHandler().HeartbeatData(ctx, viewerSessionID, req)
}

//encore:api public method=DELETE path=/api/viewer-sessions/:viewerSessionID
func CloseViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, error) {
	return runtimeHandler().CloseViewerSessionData(ctx, viewerSessionID, req)
}

//encore:api public method=DELETE path=/api/pod-sessions/:podSessionID
func ClosePodSession(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, error) {
	return runtimeHandler().ClosePodSessionData(ctx, podSessionID, req)
}

//encore:api public method=GET path=/api/pod-sessions/:podSessionID
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

type unavailableViewerService struct{}

func (unavailableViewerService) ListPVCs(_ context.Context, _ string) ([]domain.PVC, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) CreatePVC(_ context.Context, _ session.CreatePVCInput) (*domain.PVC, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) DeletePVC(_ context.Context, _ session.DeletePVCInput) (*domain.PVC, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) ExpandPVC(_ context.Context, _ session.ExpandPVCInput) (*domain.PVC, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) ListStorageClasses(_ context.Context) ([]domain.StorageClass, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) CreateViewerSession(
	_ context.Context,
	_ session.CreateViewerSessionInput,
) (*domain.ViewerSession, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) GetViewerSession(
	_ context.Context,
	_ string,
	_ string,
) (*domain.ViewerSession, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) IssueToken(_ context.Context, _ string, _ string) (*domain.ViewerToken, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) HeartbeatForUser(_ context.Context, _ string, _ string) (*domain.Heartbeat, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) CloseViewerSessionForUser(_ string, _ string) (*domain.ViewerSession, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) GetPodSession(_ string) (*domain.PodSession, error) {
	return nil, errRuntimeUnavailable
}

type unavailablePodService struct{}

func (unavailablePodService) ClosePodSession(_ context.Context, _ string) (*domain.PodSession, error) {
	return nil, errRuntimeUnavailable
}

type unavailableAuthService struct{}

func (unavailableAuthService) VerifyHook(_ session.HookVerifyInput) domain.FileBrowserHookVerification {
	return domain.FileBrowserHookVerification{
		Allow:  false,
		Reason: errRuntimeUnavailable.Error(),
		Scope:  "/",
	}
}

type denyAuthorizer struct{}

func (denyAuthorizer) CanListPVCs(_ context.Context, _ *authn.Principal, _ string) error {
	return errRuntimeUnavailable
}

func (denyAuthorizer) CanGetPVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return errRuntimeUnavailable
}

func (denyAuthorizer) CanCreatePVC(_ context.Context, _ *authn.Principal, _ string) error {
	return errRuntimeUnavailable
}

func (denyAuthorizer) CanDeletePVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return errRuntimeUnavailable
}

func (denyAuthorizer) CanUpdatePVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return errRuntimeUnavailable
}

func (denyAuthorizer) CanListStorageClasses(_ context.Context, _ *authn.Principal) error {
	return errRuntimeUnavailable
}
