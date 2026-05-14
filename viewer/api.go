package viewer

import (
	"context"
	"net/http"

	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
)

var defaultHandler *Handler

//encore:api public raw method=GET path=/api/pvcs
func ListPVCs(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().ListPVCs(w, req)
}

//encore:api public raw method=POST path=/api/viewer-sessions
func CreateViewerSession(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().CreateViewerSession(w, req)
}

//encore:api public raw method=GET path=/api/viewer-sessions/:viewerSessionID
func GetViewerSession(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().GetViewerSession(w, req)
}

//encore:api public raw method=POST path=/api/viewer-sessions/:viewerSessionID/token
func IssueViewerToken(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().IssueToken(w, req)
}

//encore:api public raw method=POST path=/api/viewer-sessions/:viewerSessionID/heartbeat
func HeartbeatViewerSession(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().Heartbeat(w, req)
}

//encore:api public raw method=DELETE path=/api/viewer-sessions/:viewerSessionID
func CloseViewerSession(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().CloseViewerSession(w, req)
}

//encore:api public raw method=DELETE path=/api/pod-sessions/:podSessionID
func ClosePodSession(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().ClosePodSession(w, req)
}

//encore:api public raw method=GET path=/api/pod-sessions/:podSessionID
func GetPodSession(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().GetPodSession(w, req)
}

//encore:api public raw method=POST path=/internal/filebrowser-hook/verify
func VerifyFileBrowserHook(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().VerifyFileBrowserHook(w, req)
}

//encore:api public raw method=GET path=/metrics
func Metrics(w http.ResponseWriter, req *http.Request) {
	runtimeHandler().Metrics(w, req)
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
