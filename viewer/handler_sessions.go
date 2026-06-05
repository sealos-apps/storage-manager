package viewer

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
)

func (h *Handler) createViewerSession(
	ctx context.Context,
	req *CreateViewerSessionRequest,
) (*ViewerSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPost, "/viewer-sessions", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if req.PVCName == "" {
		apiErr := apienv.NewError(400, apienv.CodeValidationError, "pvc_name is required", nil)
		h.observe(ctx, http.MethodPost, "/viewer-sessions", apiErr.Status, start)
		return nil, apiErr
	}
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, req.Namespace, "/viewer-sessions", req.PVCName)
	if apiErr != nil {
		h.observe(ctx, http.MethodPost, "/viewer-sessions", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanGetPVC(ctx, principal, op.namespace, req.PVCName); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
			h.recordAudit(ctx, auditDecision{
				decision:         "deny",
				denyReason:       "ssar_denied",
				mode:             operationModeUser,
				namespace:        op.namespace,
				namespaceAllowed: true,
				principal:        principal,
				pvcName:          req.PVCName,
				route:            "/viewer-sessions",
			})
			h.observe(ctx, http.MethodPost, "/viewer-sessions", apiErr.Status, start)
			return nil, apiErr
		}
	}
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       op.mode == operationModeAdmin,
		decision:           "allow",
		identityMethod:     identityMethodForOperation(op),
		implicitElevation:  op.implicitElevation,
		kubernetesUsername: op.admin.KubernetesUsername,
		mode:               op.mode,
		namespace:          op.namespace,
		namespaceAllowed:   op.namespaceAllowed,
		principal:          principal,
		pvcName:            req.PVCName,
		route:              "/viewer-sessions",
	})
	viewerSession, createErr := op.kubeService.CreateViewerSession(ctx, session.CreateViewerSessionInput{
		AdminContext: op.adminContext,
		Namespace:    op.namespace,
		PVCName:      req.PVCName,
		UserID:       principal.ID,
	})
	if createErr != nil {
		apiErr := apienv.FromError(createErr)
		h.observe(ctx, http.MethodPost, "/viewer-sessions", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/viewer-sessions", http.StatusCreated, start)
	return &ViewerSessionResponse{
		ViewerSession: viewerSession,
	}, nil
}

func (h *Handler) getViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/viewer-sessions/:id", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	viewerSession, getErr := h.viewers.GetViewerSession(ctx, viewerSessionID, principal.ID)
	if getErr != nil {
		apiErr := apienv.FromError(getErr)
		h.observe(ctx, http.MethodGet, "/viewer-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	if err := h.authorizePodSessionPVC(ctx, principal, viewerSession.PodSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodGet, "/viewer-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/viewer-sessions/:id", http.StatusOK, start)
	return &ViewerSessionResponse{ViewerSession: viewerSession}, nil
}

func (h *Handler) issueToken(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerTokenResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPost, "/viewer-sessions/:id/token", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authorizeViewerSessionPVC(ctx, principal, viewerSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodPost, "/viewer-sessions/:id/token", apiErr.Status, start)
		return nil, apiErr
	}
	token, issueErr := h.viewers.IssueToken(ctx, viewerSessionID, principal.ID)
	if issueErr != nil {
		apiErr := apienv.FromError(issueErr)
		h.observe(ctx, http.MethodPost, "/viewer-sessions/:id/token", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/viewer-sessions/:id/token", http.StatusOK, start)
	return &ViewerTokenResponse{
		CacheControl: "no-store",
		Pragma:       "no-cache",
		ViewerToken:  token,
	}, nil
}

func (h *Handler) heartbeat(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*HeartbeatResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPost, "/viewer-sessions/:id/heartbeat", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authorizeViewerSessionPVC(ctx, principal, viewerSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodPost, "/viewer-sessions/:id/heartbeat", apiErr.Status, start)
		return nil, apiErr
	}
	heartbeat, heartbeatErr := h.viewers.HeartbeatForUser(ctx, viewerSessionID, principal.ID)
	if heartbeatErr != nil {
		apiErr := apienv.FromError(heartbeatErr)
		h.observe(ctx, http.MethodPost, "/viewer-sessions/:id/heartbeat", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/viewer-sessions/:id/heartbeat", http.StatusOK, start)
	return &HeartbeatResponse{Heartbeat: heartbeat}, nil
}

func (h *Handler) closeViewerSession(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodDelete, "/viewer-sessions/:id", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authorizeViewerSessionPVC(ctx, principal, viewerSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodDelete, "/viewer-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	viewerSession, closeErr := h.viewers.CloseViewerSessionForUser(viewerSessionID, principal.ID)
	if closeErr != nil {
		apiErr := apienv.FromError(closeErr)
		h.observe(ctx, http.MethodDelete, "/viewer-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodDelete, "/viewer-sessions/:id", http.StatusOK, start)
	return &ViewerSessionResponse{ViewerSession: viewerSession}, nil
}

func (h *Handler) closePodSession(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodDelete, "/pod-sessions/:id", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authorizePodSessionPVC(ctx, principal, podSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodDelete, "/pod-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	podSession, closeErr := h.pods.ClosePodSession(ctx, podSessionID)
	if closeErr != nil {
		apiErr := apienv.FromError(closeErr)
		h.observe(ctx, http.MethodDelete, "/pod-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodDelete, "/pod-sessions/:id", http.StatusOK, start)
	return &PodSessionResponse{PodSession: podSession}, nil
}

func (h *Handler) getPodSession(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/pod-sessions/:id", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authorizePodSessionPVC(ctx, principal, podSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodGet, "/pod-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	podSession, getErr := h.viewers.GetPodSession(podSessionID)
	if getErr != nil {
		apiErr := apienv.FromError(getErr)
		h.observe(ctx, http.MethodGet, "/pod-sessions/:id", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/pod-sessions/:id", http.StatusOK, start)
	return &PodSessionResponse{PodSession: podSession}, nil
}

func (h *Handler) verifyFileBrowserHook(
	ctx context.Context,
	req *VerifyFileBrowserHookRequest,
) (*FileBrowserHookVerificationResponse, *apienv.Error) {
	start := time.Now()
	ctx, finish := h.recorder.TraceOperation(ctx,
		"filebrowser.verify_hook",
		slog.String("pod_session_id", req.PodSessionID),
		slog.String("viewer_pod_name", req.ViewerPodName),
	)
	defer finish(nil)

	result := h.auth.VerifyHook(session.HookVerifyInput{
		HookClientToken: strings.TrimPrefix(req.Authorization, "Bearer "),
		PodSessionID:    req.PodSessionID,
		ViewerPodName:   req.ViewerPodName,
		Username:        req.Username,
		AuthRequestID:   req.AuthRequestID,
		PasswordHash:    req.PasswordHash,
	})
	h.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "filebrowser.hook_verified",
		slog.String("pod_session_id", req.PodSessionID),
		slog.Bool("allowed", result.Allow),
		slog.String("reason", result.Reason),
	)
	h.observe(ctx, http.MethodPost, "/internal/filebrowser-hook/verify", http.StatusOK, start)
	return &FileBrowserHookVerificationResponse{FileBrowserHookVerification: result}, nil
}
