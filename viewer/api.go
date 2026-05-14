package viewer

import (
	"context"

	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
)

var defaultHandler *Handler

//encore:api public method=GET path=/api/pvcs
func ListPVCs(ctx context.Context, req *ListPVCsRequest) (*ListPVCsResponse, error) {
	return runtimeHandler().ListPVCsData(ctx, req)
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

//encore:api public method=GET path=/metrics
func Metrics(ctx context.Context) (*MetricsResponse, error) {
	return runtimeHandler().MetricsData(ctx)
}

type unavailableViewerService struct{}

func (unavailableViewerService) ListPVCs(_ context.Context, _ string) ([]domain.PVC, error) {
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

func (unavailableViewerService) HeartbeatForUser(_ string, _ string) (*domain.Heartbeat, error) {
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
