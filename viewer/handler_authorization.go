package viewer

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	corev1 "k8s.io/api/core/v1"
)

func (h *Handler) resolvePVCOperationContext(
	ctx context.Context,
	principal *authn.Principal,
	requestedNamespace string,
	route string,
	pvcName string,
) (*operationContext, *apienv.Error) {
	namespace := strings.TrimSpace(requestedNamespace)
	if namespace == "" {
		namespace = principal.Namespace
	}
	if namespace == principal.Namespace {
		return &operationContext{
			kubeService:      h.viewers,
			mode:             operationModeUser,
			namespace:        namespace,
			namespaceAllowed: true,
			principal:        principal,
		}, nil
	}
	adminResult, adminErr := h.checkAdmin(ctx, principal)
	ownNamespaceAllowed := adminErr == nil && isAdminInOwnNamespace(principal, adminResult)
	if adminErr != nil || !ownNamespaceAllowed {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       adminErr == nil && adminResult.Allowed,
			authorizationKind:  "kubeconfig",
			decision:           "deny",
			denyReason:         adminCapabilityDenyReason(adminErr, adminResult.Reason, ownNamespaceAllowed),
			executionKind:      "management_service_account",
			identityMethod:     "kubeconfig_context+self_subject_review",
			implicitElevation:  true,
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          namespace,
			namespaceAllowed:   false,
			principal:          principal,
			pvcName:            pvcName,
			route:              route,
		})
		return nil, apienv.NewError(403, apienv.CodeAdminAccessDenied, "Admin access denied", nil)
	}
	namespaces, err := h.viewers.ListNamespaces(ctx)
	if err != nil {
		return nil, apienv.FromError(err)
	}
	namespaceAllowed := isAdminNamespaceAllowed(namespaces, principal.Namespace, namespace)
	if !namespaceAllowed {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       true,
			authorizationKind:  "kubeconfig",
			decision:           "deny",
			denyReason:         "namespace_not_allowed",
			executionKind:      "management_service_account",
			identityMethod:     "kubeconfig_context+self_subject_review",
			implicitElevation:  true,
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          namespace,
			namespaceAllowed:   false,
			principal:          principal,
			pvcName:            pvcName,
			route:              route,
		})
		return nil, apienv.NewError(403, apienv.CodePVCAccessDenied, "Namespace is not allowed for admin PVC access", nil)
	}
	return &operationContext{
		admin:             adminResult,
		adminContext:      true,
		implicitElevation: true,
		kubeService:       h.viewers,
		mode:              operationModeAdmin,
		namespace:         namespace,
		namespaceAllowed:  true,
		principal:         principal,
	}, nil
}

func (h *Handler) checkAdmin(ctx context.Context, principal *authn.Principal) (AdminAuthorizationResult, error) {
	if h.adminAuthz == nil {
		return AdminAuthorizationResult{Reason: "not_configured"}, errors.New("admin access denied")
	}
	return h.adminAuthz.CanAdmin(ctx, principal)
}

func isAdminInOwnNamespace(principal *authn.Principal, result AdminAuthorizationResult) bool {
	if principal == nil || !result.Allowed {
		return false
	}
	return strings.TrimSpace(principal.Namespace) != "" &&
		strings.TrimSpace(principal.Namespace) == strings.TrimSpace(result.AllowedNamespace)
}

func allowedAdminNamespaces(namespaces []corev1.Namespace, currentNamespace string) []domain.Namespace {
	items := make([]domain.Namespace, 0, len(namespaces)+1)
	seen := map[string]struct{}{}
	currentNamespace = strings.TrimSpace(currentNamespace)
	if currentNamespace != "" {
		seen[currentNamespace] = struct{}{}
		items = append(items, domain.Namespace{Name: currentNamespace, IsCurrentContext: true})
	}
	for _, namespace := range namespaces {
		name := strings.TrimSpace(namespace.Name)
		if name == "" {
			continue
		}
		// Sealos user namespaces use the ns- prefix. Admin PVC browsing only exposes
		// system namespaces so the dropdown stays bounded and never lists other users.
		if isUserNamespace(name) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		items = append(items, domain.Namespace{Name: name, IsCurrentContext: name == currentNamespace})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsCurrentContext != items[j].IsCurrentContext {
			return items[i].IsCurrentContext
		}
		return items[i].Name < items[j].Name
	})
	return items
}

func isAdminNamespaceAllowed(namespaces []corev1.Namespace, currentNamespace string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	return slices.ContainsFunc(allowedAdminNamespaces(namespaces, currentNamespace), func(namespace domain.Namespace) bool {
		return namespace.Name == target
	})
}

func (h *Handler) observe(ctx context.Context, method string, route string, status int, start time.Time) {
	h.recorder.ObserveHTTP(ctx, method, route, status, time.Since(start))
}

func (h *Handler) recordAudit(ctx context.Context, decision auditDecision) {
	if decision.authorizationKind == "" {
		decision.authorizationKind = "kubeconfig"
	}
	if decision.executionKind == "" {
		decision.executionKind = "management_service_account"
	}
	if decision.identityMethod == "" {
		decision.identityMethod = "kubeconfig_context"
	}
	if decision.decision == "" {
		decision.decision = "allow"
	}
	attrs := []slog.Attr{
		slog.String("route", decision.route),
		slog.String("namespace", decision.namespace),
		slog.String("auth_mode", string(decision.mode)),
		slog.Bool("implicit_admin_elevation", decision.implicitElevation),
		slog.String("identity_method", decision.identityMethod),
		slog.String("authorization_credential", decision.authorizationKind),
		slog.String("execution_credential", decision.executionKind),
		slog.Bool("admin_allowed", decision.adminAllowed),
		slog.Bool("namespace_allowed", decision.namespaceAllowed),
		slog.String("decision", decision.decision),
	}
	if decision.denyReason != "" {
		attrs = append(attrs, slog.String("deny_reason", decision.denyReason))
	}
	if decision.principal != nil {
		attrs = append(attrs,
			slog.String("principal_id", decision.principal.ID),
			slog.String("principal_context_name", decision.principal.ContextName),
			slog.String("principal_namespace", decision.principal.Namespace),
		)
	}
	if decision.kubernetesUsername != "" {
		attrs = append(attrs, slog.String("kubernetes_username", decision.kubernetesUsername))
	}
	if decision.pvcName != "" {
		attrs = append(attrs, slog.String("pvc_name", decision.pvcName))
	}
	if decision.viewerSessionID != "" {
		attrs = append(attrs, slog.String("viewer_session_id", decision.viewerSessionID))
	}
	if decision.podSessionID != "" {
		attrs = append(attrs, slog.String("pod_session_id", decision.podSessionID))
	}
	ctx, finish := h.recorder.TraceOperation(ctx, "authorization.audit", attrs...)
	defer finish(nil)
	h.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "authorization.audit", attrs...)
}

func allowDeny(allowed bool) string {
	if allowed {
		return "allow"
	}
	return "deny"
}

func denyReason(err error, reason string) string {
	if err == nil {
		return ""
	}
	if strings.TrimSpace(reason) != "" {
		return reason
	}
	return "denied"
}

func identityMethodForOperation(op *operationContext) string {
	if op != nil && op.mode == operationModeAdmin {
		return "kubeconfig_context+self_subject_review"
	}
	return "kubeconfig_context"
}

func (h *Handler) authenticateRequest(req interface{ authorizationHeader() string }) (*authn.Principal, *apienv.Error) {
	principal, err := authn.PrincipalFromAuthorization(req.authorizationHeader())
	if err != nil {
		return nil, apienv.NewError(401, apienv.CodeUnauthorized, "Unauthorized", nil)
	}
	if h.debug.Enabled && strings.TrimSpace(h.debug.ForcedNamespace) != "" {
		principal.Namespace = strings.TrimSpace(h.debug.ForcedNamespace)
	}
	return principal, nil
}

func (req *AuthenticatedRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *StorageQuotaRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *ListPVCsRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *CreatePVCRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *ExpandPVCRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *PVCYAMLRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *StorageClassYAMLRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *StorageClassMetadataRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (req *CreateViewerSessionRequest) authorizationHeader() string {
	if req == nil {
		return ""
	}
	return req.Authorization
}

func (h *Handler) authorizeViewerSessionPVC(
	ctx context.Context,
	principal *authn.Principal,
	viewerSessionID string,
) error {
	session, err := h.viewers.GetViewerSession(ctx, viewerSessionID, principal.ID)
	if err != nil {
		return err
	}
	if session.Namespace == "" || session.PVCName == "" {
		return apienv.NewError(500, apienv.CodeInternal, "Viewer session is missing PVC identity", nil)
	}
	if session.AdminContext {
		return h.authorizeAdminSessionPVC(ctx, principal, session.Namespace, session.PVCName, viewerSessionID, "")
	}
	if err := h.authz.CanGetPVC(ctx, principal, session.Namespace, session.PVCName); err != nil {
		h.recordAudit(ctx, auditDecision{
			decision:         "deny",
			denyReason:       "ssar_denied",
			mode:             operationModeUser,
			namespace:        session.Namespace,
			namespaceAllowed: true,
			principal:        principal,
			pvcName:          session.PVCName,
			viewerSessionID:  viewerSessionID,
			route:            "/viewer-sessions/:id",
		})
		return apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
	}
	h.recordAudit(ctx, auditDecision{
		decision:         "allow",
		mode:             operationModeUser,
		namespace:        session.Namespace,
		namespaceAllowed: true,
		principal:        principal,
		pvcName:          session.PVCName,
		viewerSessionID:  viewerSessionID,
		route:            "/viewer-sessions/:id",
	})
	return nil
}

func (h *Handler) authorizePodSessionPVC(
	ctx context.Context,
	principal *authn.Principal,
	podSessionID string,
) error {
	podSession, err := h.viewers.GetPodSession(podSessionID)
	if err != nil {
		return err
	}
	if podSession.AdminContext {
		return h.authorizeAdminSessionPVC(ctx, principal, podSession.Namespace, podSession.PVCName, "", podSessionID)
	}
	if err := h.authz.CanGetPVC(ctx, principal, podSession.Namespace, podSession.PVCName); err != nil {
		h.recordAudit(ctx, auditDecision{
			decision:         "deny",
			denyReason:       "ssar_denied",
			mode:             operationModeUser,
			namespace:        podSession.Namespace,
			namespaceAllowed: true,
			podSessionID:     podSessionID,
			principal:        principal,
			pvcName:          podSession.PVCName,
			route:            "/pod-sessions/:id",
		})
		return apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
	}
	h.recordAudit(ctx, auditDecision{
		decision:         "allow",
		mode:             operationModeUser,
		namespace:        podSession.Namespace,
		namespaceAllowed: true,
		podSessionID:     podSessionID,
		principal:        principal,
		pvcName:          podSession.PVCName,
		route:            "/pod-sessions/:id",
	})
	return nil
}

func (h *Handler) authorizeAdminSessionPVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
	pvcName string,
	viewerSessionID string,
	podSessionID string,
) error {
	adminResult, adminErr := h.checkAdmin(ctx, principal)
	ownNamespaceAllowed := adminErr == nil && isAdminInOwnNamespace(principal, adminResult)
	if adminErr != nil || !ownNamespaceAllowed {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       adminErr == nil && adminResult.Allowed,
			decision:           "deny",
			denyReason:         adminCapabilityDenyReason(adminErr, adminResult.Reason, ownNamespaceAllowed),
			identityMethod:     "kubeconfig_context+self_subject_review",
			implicitElevation:  true,
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          namespace,
			namespaceAllowed:   false,
			podSessionID:       podSessionID,
			principal:          principal,
			pvcName:            pvcName,
			route:              sessionAuditRoute(viewerSessionID, podSessionID),
			viewerSessionID:    viewerSessionID,
		})
		return apienv.NewError(403, apienv.CodeAdminAccessDenied, "Admin access denied", nil)
	}
	namespaces, err := h.viewers.ListNamespaces(ctx)
	if err != nil {
		return err
	}
	namespaceAllowed := isAdminNamespaceAllowed(namespaces, principal.Namespace, namespace)
	if !namespaceAllowed {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       true,
			decision:           "deny",
			denyReason:         "namespace_not_allowed",
			identityMethod:     "kubeconfig_context+self_subject_review",
			implicitElevation:  true,
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          namespace,
			namespaceAllowed:   false,
			podSessionID:       podSessionID,
			principal:          principal,
			pvcName:            pvcName,
			route:              sessionAuditRoute(viewerSessionID, podSessionID),
			viewerSessionID:    viewerSessionID,
		})
		return apienv.NewError(403, apienv.CodePVCAccessDenied, "Namespace is not allowed for admin PVC access", nil)
	}
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       true,
		decision:           "allow",
		identityMethod:     "kubeconfig_context+self_subject_review",
		implicitElevation:  true,
		kubernetesUsername: adminResult.KubernetesUsername,
		mode:               operationModeAdmin,
		namespace:          namespace,
		namespaceAllowed:   true,
		podSessionID:       podSessionID,
		principal:          principal,
		pvcName:            pvcName,
		route:              sessionAuditRoute(viewerSessionID, podSessionID),
		viewerSessionID:    viewerSessionID,
	})
	return nil
}

func sessionAuditRoute(viewerSessionID string, podSessionID string) string {
	if viewerSessionID != "" {
		return "/viewer-sessions/:id"
	}
	if podSessionID != "" {
		return "/pod-sessions/:id"
	}
	return "/session"
}

func pathID(path string, prefix string) string {
	return strings.Trim(strings.TrimPrefix(path, prefix), "/")
}

func pvcPathParams(path string) (string, string) {
	remainder := strings.Trim(strings.TrimPrefix(path, "/pvcs/"), "/")
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func expandPVCPathParams(path string) (string, string) {
	remainder := strings.TrimSuffix(strings.Trim(strings.TrimPrefix(path, "/pvcs/"), "/"), "/expand")
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func pvcYAMLPathParams(path string) (string, string) {
	return pvcSuffixedPathParams(path, "/yaml")
}

func pvcDescribePathParams(path string) (string, string) {
	return pvcSuffixedPathParams(path, "/describe")
}

func pvcSuffixedPathParams(path string, suffix string) (string, string) {
	remainder := strings.TrimSuffix(strings.Trim(strings.TrimPrefix(path, "/pvcs/"), "/"), suffix)
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
