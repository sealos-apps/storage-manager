package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	"github.com/nixieboluo/sealos-storage-manager/viewer"
)

type fakeViewerService struct{}

func (fakeViewerService) ListPVCs(_ context.Context, _ string) ([]domain.PVC, error) {
	return []domain.PVC{}, nil
}

func (fakeViewerService) CreateViewerSession(
	_ context.Context,
	_ session.CreateViewerSessionInput,
) (*domain.ViewerSession, error) {
	return &domain.ViewerSession{}, nil
}

func (fakeViewerService) GetViewerSession(
	_ context.Context,
	_ string,
	_ string,
) (*domain.ViewerSession, error) {
	return &domain.ViewerSession{}, nil
}

func (fakeViewerService) IssueToken(_ context.Context, _ string, _ string) (*domain.ViewerToken, error) {
	return &domain.ViewerToken{}, nil
}

func (fakeViewerService) HeartbeatForUser(_ string, _ string) (*domain.Heartbeat, error) {
	return &domain.Heartbeat{}, nil
}

func (fakeViewerService) CloseViewerSessionForUser(_ string, _ string) (*domain.ViewerSession, error) {
	return &domain.ViewerSession{}, nil
}

func (fakeViewerService) GetPodSession(_ string) (*domain.PodSession, error) {
	return &domain.PodSession{}, nil
}

type fakePodService struct{}

func (fakePodService) ClosePodSession(_ context.Context, _ string) (*domain.PodSession, error) {
	return &domain.PodSession{}, nil
}

type fakeAuthService struct{}

func (fakeAuthService) VerifyHook(_ session.HookVerifyInput) domain.FileBrowserHookVerification {
	return domain.FileBrowserHookVerification{}
}

type allowAuthorizer struct{}

func (allowAuthorizer) CanListPVCs(_ context.Context, _ *authn.Principal, _ string) error {
	return nil
}

func (allowAuthorizer) CanGetPVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return nil
}

func TestRoutesDispatchMetrics(t *testing.T) {
	t.Parallel()

	handler := viewer.NewHandler(
		fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	server := httptest.NewServer(routes(handler))
	defer server.Close()

	response, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestRoutesRejectWrongMethod(t *testing.T) {
	t.Parallel()

	handler := viewer.NewHandler(
		fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	server := httptest.NewServer(routes(handler))
	defer server.Close()

	response, err := http.Get(server.URL + "/api/viewer-sessions")
	if err != nil {
		t.Fatalf("get viewer sessions: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func testObservability() config.ObservabilityConfig {
	cfg := config.Default().Observability
	cfg.Logs.Exporter = "discard"
	cfg.Logs.Level = "error"
	return cfg
}
