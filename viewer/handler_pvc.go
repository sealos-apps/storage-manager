package viewer

import (
	"context"
	"net/http"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
)

func (h *Handler) listPVCs(ctx context.Context, req *ListPVCsRequest) (*ListPVCsResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/pvcs", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, req.Namespace, "/pvcs", "")
	if apiErr != nil {
		h.observe(ctx, http.MethodGet, "/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanListPVCs(ctx, principal, op.namespace); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
			h.recordAudit(ctx, auditDecision{
				adminAllowed:      false,
				decision:          "deny",
				denyReason:        "ssar_denied",
				implicitElevation: false,
				mode:              operationModeUser,
				namespace:         op.namespace,
				namespaceAllowed:  true,
				principal:         principal,
				route:             "/pvcs",
			})
			h.observe(ctx, http.MethodGet, "/pvcs", apiErr.Status, start)
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
		route:              "/pvcs",
	})
	items, listErr := op.kubeService.ListPVCs(ctx, op.namespace)
	if listErr != nil {
		apiErr := apienv.FromError(listErr)
		h.observe(ctx, http.MethodGet, "/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	if items == nil {
		items = []domain.PVC{}
	}
	h.observe(ctx, http.MethodGet, "/pvcs", http.StatusOK, start)
	return &ListPVCsResponse{PVCList: PVCList{Items: items}}, nil
}

func (h *Handler) getContext(ctx context.Context, req *AuthenticatedRequest) (*ContextResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/context", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authz.CanListPVCs(ctx, principal, principal.Namespace); err != nil {
		apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "Namespace access denied", nil)
		h.observe(ctx, http.MethodGet, "/context", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/context", http.StatusOK, start)
	return &ContextResponse{
		Context: ViewerContext{
			ContextName: principal.ContextName,
			Namespace:   principal.Namespace,
		},
	}, nil
}

func (h *Handler) createPVC(ctx context.Context, req *CreatePVCRequest) (*PVCResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPost, "/pvcs", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, req.Namespace, "/pvcs", req.Name)
	if apiErr != nil {
		h.observe(ctx, http.MethodPost, "/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanCreatePVC(ctx, principal, op.namespace); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCCreateForbidden, "PVC create access denied", nil)
			h.recordAudit(ctx, auditDecision{
				decision:         "deny",
				denyReason:       "ssar_denied",
				mode:             operationModeUser,
				namespace:        op.namespace,
				namespaceAllowed: true,
				principal:        principal,
				pvcName:          req.Name,
				route:            "/pvcs",
			})
			h.observe(ctx, http.MethodPost, "/pvcs", apiErr.Status, start)
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
		pvcName:            req.Name,
		route:              "/pvcs",
	})
	pvc, createErr := op.kubeService.CreatePVC(ctx, session.CreatePVCInput{
		Namespace:        op.namespace,
		Name:             req.Name,
		Capacity:         req.Capacity,
		CapacityBytes:    req.CapacityBytes,
		AccessModes:      req.AccessModes,
		StorageClassName: req.StorageClassName,
	})
	if createErr != nil {
		apiErr := apienv.FromError(createErr)
		h.observe(ctx, http.MethodPost, "/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/pvcs", http.StatusCreated, start)
	return &PVCResponse{PVC: pvc}, nil
}

func (h *Handler) deletePVC(
	ctx context.Context,
	namespace string,
	name string,
	req *AuthenticatedRequest,
) (*PVCResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodDelete, "/pvcs/:namespace/:name", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, namespace, "/pvcs/:namespace/:name", name)
	if apiErr != nil {
		h.observe(ctx, http.MethodDelete, "/pvcs/:namespace/:name", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanDeletePVC(ctx, principal, op.namespace, name); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCDeleteForbidden, "PVC delete access denied", nil)
			h.recordAudit(ctx, auditDecision{
				decision:         "deny",
				denyReason:       "ssar_denied",
				mode:             operationModeUser,
				namespace:        op.namespace,
				namespaceAllowed: true,
				principal:        principal,
				pvcName:          name,
				route:            "/pvcs/:namespace/:name",
			})
			h.observe(ctx, http.MethodDelete, "/pvcs/:namespace/:name", apiErr.Status, start)
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
		pvcName:            name,
		route:              "/pvcs/:namespace/:name",
	})
	pvc, deleteErr := op.kubeService.DeletePVC(ctx, session.DeletePVCInput{
		Namespace: op.namespace,
		Name:      name,
	})
	if deleteErr != nil {
		apiErr := apienv.FromError(deleteErr)
		h.observe(ctx, http.MethodDelete, "/pvcs/:namespace/:name", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodDelete, "/pvcs/:namespace/:name", http.StatusOK, start)
	return &PVCResponse{PVC: pvc}, nil
}

func (h *Handler) expandPVC(
	ctx context.Context,
	namespace string,
	name string,
	req *ExpandPVCRequest,
) (*PVCResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPost, "/pvcs/:namespace/:name/expand", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, namespace, "/pvcs/:namespace/:name/expand", name)
	if apiErr != nil {
		h.observe(ctx, http.MethodPost, "/pvcs/:namespace/:name/expand", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanUpdatePVC(ctx, principal, op.namespace, name); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCExpandForbidden, "PVC expand access denied", nil)
			h.recordAudit(ctx, auditDecision{
				decision:         "deny",
				denyReason:       "ssar_denied",
				mode:             operationModeUser,
				namespace:        op.namespace,
				namespaceAllowed: true,
				principal:        principal,
				pvcName:          name,
				route:            "/pvcs/:namespace/:name/expand",
			})
			h.observe(ctx, http.MethodPost, "/pvcs/:namespace/:name/expand", apiErr.Status, start)
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
		pvcName:            name,
		route:              "/pvcs/:namespace/:name/expand",
	})
	pvc, expandErr := op.kubeService.ExpandPVC(ctx, session.ExpandPVCInput{
		Namespace:     op.namespace,
		Name:          name,
		Capacity:      req.Capacity,
		CapacityBytes: req.CapacityBytes,
	})
	if expandErr != nil {
		apiErr := apienv.FromError(expandErr)
		h.observe(ctx, http.MethodPost, "/pvcs/:namespace/:name/expand", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/pvcs/:namespace/:name/expand", http.StatusOK, start)
	return &PVCResponse{PVC: pvc}, nil
}

func (h *Handler) listStorageClasses(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListStorageClassesResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/storage-classes", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if err := h.authz.CanListStorageClasses(ctx, principal); err != nil {
		apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "Storage class access denied", nil)
		h.observe(ctx, http.MethodGet, "/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	items, listErr := h.viewers.ListStorageClasses(ctx)
	if listErr != nil {
		apiErr := apienv.FromError(listErr)
		h.observe(ctx, http.MethodGet, "/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	if items == nil {
		items = []domain.StorageClass{}
	}
	h.observe(ctx, http.MethodGet, "/storage-classes", http.StatusOK, start)
	return &ListStorageClassesResponse{StorageClassList: StorageClassList{Items: items}}, nil
}
