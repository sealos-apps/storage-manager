package session

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
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
) (viewerToken *domain.ViewerToken, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"auth.issue_token",
		slog.String("viewer_session_id", viewer.ID),
		slog.String("pod_session_id", pod.ID),
	)
	defer func() {
		finish(err)
	}()

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
	s.recorder.ObserveAuthRequest("created")

	password := authID + "." + secret
	loginCtx, finishLogin := s.recorder.TraceOperation(ctx,
		"filebrowser.login",
		slog.String("viewer_session_id", viewer.ID),
		slog.String("pod_session_id", pod.ID),
		slog.String("viewer_host", viewerHost(pod.ViewerURL)),
	)
	loginURL := s.fileBrowserLoginURL(pod)
	token, err := s.login.Login(loginCtx, loginURL, viewer.ID, password)
	finishLogin(err)
	if err != nil {
		s.recorder.ObserveFileBrowserLogin("error")
		return nil, apienv.NewError(502, apienv.CodeFileBrowserLoginFailed, "File Browser login failed", nil)
	}
	s.recorder.ObserveFileBrowserLogin("success")

	tokenHash := filebrowser.HashSecret(token)
	expiresAt := now.Add(s.cfg.Viewer.FileBrowser.TokenTTL)
	s.store.PutTokenRecord(&domain.TokenRecord{
		TokenHash:       tokenHash,
		ViewerSessionID: viewer.ID,
		PodSessionID:    pod.ID,
		IssuedAt:        now,
		ExpiresAt:       expiresAt,
	})
	s.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "viewer.token_issued",
		slog.String("viewer_session_id", viewer.ID),
		slog.String("pod_session_id", pod.ID),
		slog.Time("expires_at", expiresAt),
	)
	return &domain.ViewerToken{
		ViewerSessionID: viewer.ID,
		PodSessionID:    pod.ID,
		ViewerURL:       pod.ViewerURL,
		Token:           token,
		TokenType:       "Bearer",
		ExpiresAt:       expiresAt,
	}, nil
}

func (s *AuthService) fileBrowserLoginURL(pod *domain.PodSession) string {
	if s.cfg.Viewer.FileBrowser.LoginURLMode == "public" {
		return pod.ViewerURL
	}
	if pod.InternalViewerURL != "" {
		return pod.InternalViewerURL
	}
	return pod.ViewerURL
}

func (s *AuthService) VerifyHook(input HookVerifyInput) domain.FileBrowserHookVerification {
	if !constantTimeEqual(input.HookClientToken, s.cfg.Viewer.HookClientToken) {
		s.recorder.ObserveAuthRequest("denied")
		return deny("invalid hook token")
	}
	now := s.now()
	req, ok := s.store.ConsumeAuthRequest(input.AuthRequestID, input.PasswordHash, now)
	if !ok {
		s.recorder.ObserveAuthRequest("denied")
		return deny("invalid auth request")
	}
	if req.ViewerSessionID != input.Username || req.PodSessionID != input.PodSessionID {
		s.recorder.ObserveAuthRequest("denied")
		return deny("auth request scope mismatch")
	}
	viewer, ok := s.store.GetViewerSession(req.ViewerSessionID, now)
	if !ok || viewer.Status == domain.ViewerStatusClosed || viewer.Status == domain.ViewerStatusExpired {
		s.recorder.ObserveAuthRequest("denied")
		return deny("viewer session is not active")
	}
	pod, ok := s.store.GetPodSession(req.PodSessionID, now)
	if !ok || pod.Status != domain.PodStatusReady {
		s.recorder.ObserveAuthRequest("denied")
		return deny("pod session is not ready")
	}

	s.recorder.ObserveAuthRequest("consumed")
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

func viewerHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Host
}
