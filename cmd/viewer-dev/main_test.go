package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nixieboluo/sealos-stroage-manager/internal/authn"
	"github.com/nixieboluo/sealos-stroage-manager/internal/config"
	"github.com/nixieboluo/sealos-stroage-manager/internal/domain"
	"github.com/nixieboluo/sealos-stroage-manager/internal/observability"
	"github.com/nixieboluo/sealos-stroage-manager/internal/session"
	"github.com/nixieboluo/sealos-stroage-manager/viewer"
)

type fakeViewerService struct{}

func (fakeViewerService) ListPVCs(ctx context.Context, namespace string) ([]domain.PVC, error) {
	return []domain.PVC{}, nil
}

func (fakeViewerService) CreateViewerSession(
	ctx context.Context,
	input session.CreateViewerSessionInput,
) (*domain.ViewerSession, error) {
	return &domain.ViewerSession{}, nil
}

func (fakeViewerService) GetViewerSession(
	ctx context.Context,
	id string,
	userID string,
) (*domain.ViewerSession, error) {
	return &domain.ViewerSession{}, nil
}

func (fakeViewerService) IssueToken(ctx context.Context, id string, userID string) (*domain.ViewerToken, error) {
	return &domain.ViewerToken{}, nil
}

func (fakeViewerService) HeartbeatForUser(id string, userID string) (*domain.Heartbeat, error) {
	return &domain.Heartbeat{}, nil
}

func (fakeViewerService) CloseViewerSessionForUser(id string, userID string) (*domain.ViewerSession, error) {
	return &domain.ViewerSession{}, nil
}

func (fakeViewerService) GetPodSession(id string) (*domain.PodSession, error) {
	return &domain.PodSession{}, nil
}

type fakePodService struct{}

func (fakePodService) ClosePodSession(ctx context.Context, id string) (*domain.PodSession, error) {
	return &domain.PodSession{}, nil
}

type fakeAuthService struct{}

func (fakeAuthService) VerifyHook(input session.HookVerifyInput) domain.FileBrowserHookVerification {
	return domain.FileBrowserHookVerification{}
}

type allowAuthorizer struct{}

func (allowAuthorizer) CanListPVCs(ctx context.Context, principal *authn.Principal, namespace string) error {
	return nil
}

func (allowAuthorizer) CanGetPVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
	name string,
) error {
	return nil
}

func TestRoutesDispatchMetrics(t *testing.T) {
	t.Parallel()

	handler := viewer.NewHandler(
		fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		observability.New(config.ObservabilityConfig{LogLevel: "error"}, nil),
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
		observability.New(config.ObservabilityConfig{LogLevel: "error"}, nil),
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
