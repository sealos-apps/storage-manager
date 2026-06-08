package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
)

type fakeLogin struct {
	viewerURL string
	username  string
	password  string
	token     string
	err       error
}

func (f *fakeLogin) Login(_ context.Context, viewerURL string, username string, password string) (string, error) {
	f.viewerURL = viewerURL
	f.username = username
	f.password = password
	if f.err != nil {
		return "", f.err
	}
	return f.token, nil
}

func TestIssueTokenCreatesOneTimeAuthRequestAndTokenRecord(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	login := &fakeLogin{token: "fb-token"}
	auth := NewAuthService(cfg, store, login, observability.MustNew(cfg.Observability, nil))
	auth.now = fixedNow
	viewer := &domain.ViewerSession{ID: "vs_1", Permission: domain.ModeReadWrite}
	pod := &domain.PodSession{
		ID:                "ps_1",
		ViewerURL:         "https://viewer.example.test",
		InternalViewerURL: "http://viewer-ps-1.default.svc.cluster.local:80",
		Status:            domain.PodStatusReady,
	}

	token, err := auth.IssueToken(t.Context(), viewer, pod)
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}
	if token.Token != "fb-token" || token.TokenType != "Bearer" {
		t.Fatalf("token = %#v", token)
	}
	if token.ViewerURL != "https://viewer.example.test" {
		t.Fatalf("token viewer URL = %q", token.ViewerURL)
	}
	if login.viewerURL != "http://viewer-ps-1.default.svc.cluster.local:80" {
		t.Fatalf("login viewer URL = %q", login.viewerURL)
	}
	if login.username != "vs_1" {
		t.Fatalf("username = %q", login.username)
	}
	if !strings.Contains(login.password, ".") || strings.Contains(login.password, "fb-token") {
		t.Fatalf("unexpected password format: %q", login.password)
	}
	if len(login.password) > 72 {
		t.Fatalf("password length = %d, exceeds bcrypt limit", len(login.password))
	}
}

func TestIssueTokenMapsFileBrowserLoginFailure(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	auth := NewAuthService(
		cfg,
		store,
		&fakeLogin{err: errors.New("filebrowser login returned status 403")},
		observability.MustNew(cfg.Observability, nil),
	)
	auth.now = fixedNow
	viewer := &domain.ViewerSession{ID: "vs_1", Permission: domain.ModeReadWrite}
	pod := &domain.PodSession{ID: "ps_1", ViewerURL: "http://viewer", Status: domain.PodStatusReady}

	_, err := auth.IssueToken(t.Context(), viewer, pod)

	var apiErr *apienv.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("IssueToken() error = %T %v, want apienv.Error", err, err)
	}
	if apiErr.Code != apienv.CodeFileBrowserLoginFailed || apiErr.Status != 502 {
		t.Fatalf("api error = %#v", apiErr)
	}
}

func TestVerifyHookAllowsAndConsumesAuthRequestOnce(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	auth := NewAuthService(cfg, store, &fakeLogin{token: "unused"}, observability.MustNew(cfg.Observability, nil))
	auth.now = fixedNow
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_1",
		PodSessionID: "ps_1",
		Permission:   domain.ModeReadWrite,
		Status:       domain.ViewerStatusReady,
		ExpiresAt:    fixedNow().Add(cfg.Sessions.ViewerSessionTimout),
	})
	store.PutPodSession(&domain.PodSession{
		ID:        "ps_1",
		Namespace: "default",
		PVCUID:    "uid",
		Status:    domain.PodStatusReady,
		ExpiresAt: fixedNow().Add(cfg.Sessions.PodKeepaliveGrace),
	})
	store.CreateAuthRequest(&domain.AuthRequest{
		ID:              "ar_1",
		ViewerSessionID: "vs_1",
		PodSessionID:    "ps_1",
		Username:        "vs_1",
		ExpiresAt:       fixedNow().Add(cfg.Sessions.AuthRequestTTL),
		CreatedAt:       fixedNow(),
	}, "hash")

	result := auth.VerifyHook(HookVerifyInput{
		HookClientToken: "hook-token",
		PodSessionID:    "ps_1",
		Username:        "vs_1",
		AuthRequestID:   "ar_1",
		PasswordHash:    "hash",
	})
	if !result.Allow || !result.Permissions.Create {
		t.Fatalf("verification = %#v", result)
	}
	again := auth.VerifyHook(HookVerifyInput{
		HookClientToken: "hook-token",
		PodSessionID:    "ps_1",
		Username:        "vs_1",
		AuthRequestID:   "ar_1",
		PasswordHash:    "hash",
	})
	if again.Allow {
		t.Fatal("auth request was reusable")
	}
}

func TestVerifyHookDeniesReadOnlyMutationPermissions(t *testing.T) {
	t.Parallel()

	perms := permissionsForMode(domain.ModeReadOnly)
	if perms.Create || perms.Delete || !perms.Download {
		t.Fatalf("readonly permissions = %#v", perms)
	}
}

func TestConstantTimeEqualRejectsEmptyToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "both match", a: "hook-token", b: "hook-token", want: true},
		{name: "different", a: "hook-token", b: "other", want: false},
		{name: "empty left", a: "", b: "hook-token", want: false},
		{name: "empty right", a: "hook-token", b: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := constantTimeEqual(tt.a, tt.b); got != tt.want {
				t.Fatalf("constantTimeEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
