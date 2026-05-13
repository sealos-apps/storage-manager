package session

import (
	"context"
	"strings"
	"testing"

	"github.com/nixieboluo/sealos-stroage-manager/internal/domain"
	"github.com/nixieboluo/sealos-stroage-manager/internal/observability"
	"github.com/nixieboluo/sealos-stroage-manager/internal/state"
)

type fakeLogin struct {
	viewerURL string
	username  string
	password  string
	token     string
	err       error
}

func (f *fakeLogin) Login(ctx context.Context, viewerURL string, username string, password string) (string, error) {
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
	auth := NewAuthService(cfg, store, login, observability.New(cfg.Observability, nil))
	auth.now = fixedNow
	viewer := &domain.ViewerSession{ID: "vs_1", Permission: domain.ModeReadWrite}
	pod := &domain.PodSession{ID: "ps_1", ViewerURL: "http://viewer", Status: domain.PodStatusReady}

	token, err := auth.IssueToken(context.Background(), viewer, pod)
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}
	if token.Token != "fb-token" || token.TokenType != "Bearer" {
		t.Fatalf("token = %#v", token)
	}
	if login.username != "vs_1" {
		t.Fatalf("username = %q", login.username)
	}
	if !strings.Contains(login.password, ".") || strings.Contains(login.password, "fb-token") {
		t.Fatalf("unexpected password format: %q", login.password)
	}
}

func TestVerifyHookAllowsAndConsumesAuthRequestOnce(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := state.New(cfg.Cache)
	auth := NewAuthService(cfg, store, &fakeLogin{token: "unused"}, observability.New(cfg.Observability, nil))
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
