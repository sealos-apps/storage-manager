package session

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/filebrowser"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
)

type FileBrowserLogin interface {
	Login(ctx context.Context, viewerURL string, username string, password string) (string, error)
}

type AuthService struct {
	cfg      config.Config
	store    *state.Store
	login    FileBrowserLogin
	recorder *observability.Recorder
	now      func() time.Time
}

type HookVerifyInput struct {
	HookClientToken string
	PodSessionID    string
	ViewerPodName   string
	Username        string
	AuthRequestID   string
	PasswordHash    string
}

func NewAuthService(
	cfg config.Config,
	store *state.Store,
	login FileBrowserLogin,
	recorder *observability.Recorder,
) *AuthService {
	return &AuthService{
		cfg:      cfg,
		store:    store,
		login:    login,
		recorder: recorder,
		now:      time.Now,
	}
}

func (s *AuthService) IssueToken(
	ctx context.Context,
	viewer *domain.ViewerSession,
	pod *domain.PodSession,
) (*domain.ViewerToken, error) {
	authID, err := newID("ar")
	if err != nil {
		return nil, err
	}
	secret, err := randomSecret()
	if err != nil {
		return nil, err
	}
	now := s.now()
	passwordHash := filebrowser.HashSecret(secret)
	s.store.CreateAuthRequest(&domain.AuthRequest{
		ID:              authID,
		ViewerSessionID: viewer.ID,
		PodSessionID:    pod.ID,
		Username:        viewer.ID,
		PasswordHash:    passwordHash,
		ExpiresAt:       now.Add(s.cfg.Sessions.AuthRequestTTL),
		CreatedAt:       now,
	}, passwordHash)
	s.recorder.Metrics().AuthCreated.Add(1)

	password := authID + "." + secret
	token, err := s.login.Login(ctx, pod.ViewerURL, viewer.ID, password)
	if err != nil {
		s.recorder.Metrics().FileBrowserErrors.Add(1)
		return nil, fmt.Errorf("issuing filebrowser token: %w", err)
	}
	s.recorder.Metrics().FileBrowserLogins.Add(1)

	tokenHash := filebrowser.HashSecret(token)
	expiresAt := now.Add(s.cfg.Viewer.FileBrowser.TokenTTL)
	s.store.PutTokenRecord(&domain.TokenRecord{
		TokenHash:       tokenHash,
		ViewerSessionID: viewer.ID,
		PodSessionID:    pod.ID,
		IssuedAt:        now,
		ExpiresAt:       expiresAt,
	})
	return &domain.ViewerToken{
		ViewerSessionID: viewer.ID,
		PodSessionID:    pod.ID,
		ViewerURL:       pod.ViewerURL,
		Token:           token,
		TokenType:       "Bearer",
		ExpiresAt:       expiresAt,
	}, nil
}

func (s *AuthService) VerifyHook(input HookVerifyInput) domain.FileBrowserHookVerification {
	if !constantTimeEqual(input.HookClientToken, s.cfg.Viewer.HookClientToken) {
		s.recorder.Metrics().AuthDenied.Add(1)
		return deny("invalid hook token")
	}
	now := s.now()
	req, ok := s.store.ConsumeAuthRequest(input.AuthRequestID, input.PasswordHash, now)
	if !ok {
		s.recorder.Metrics().AuthDenied.Add(1)
		return deny("invalid auth request")
	}
	if req.ViewerSessionID != input.Username || req.PodSessionID != input.PodSessionID {
		s.recorder.Metrics().AuthDenied.Add(1)
		return deny("auth request scope mismatch")
	}
	viewer, ok := s.store.GetViewerSession(req.ViewerSessionID, now)
	if !ok || viewer.Status == domain.ViewerStatusClosed || viewer.Status == domain.ViewerStatusExpired {
		s.recorder.Metrics().AuthDenied.Add(1)
		return deny("viewer session is not active")
	}
	pod, ok := s.store.GetPodSession(req.PodSessionID, now)
	if !ok || pod.Status != domain.PodStatusReady {
		s.recorder.Metrics().AuthDenied.Add(1)
		return deny("pod session is not ready")
	}

	s.recorder.Metrics().AuthConsumed.Add(1)
	return domain.FileBrowserHookVerification{
		Allow:       true,
		Reason:      "",
		Scope:       "/",
		Permissions: permissionsForMode(viewer.Permission),
	}
}

func constantTimeEqual(a string, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func permissionsForMode(mode string) domain.FileBrowserPermissions {
	permissions := domain.FileBrowserPermissions{
		Download: true,
	}
	if mode == domain.ModeReadWrite {
		permissions.Create = true
		permissions.Rename = true
		permissions.Modify = true
		permissions.Delete = true
	}
	return permissions
}

func deny(reason string) domain.FileBrowserHookVerification {
	return domain.FileBrowserHookVerification{
		Allow:       false,
		Reason:      reason,
		Scope:       "/",
		Permissions: domain.FileBrowserPermissions{},
	}
}

func randomSecret() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generating auth secret: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
