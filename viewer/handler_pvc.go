package viewer

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/accountquota"
	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	"k8s.io/apimachinery/pkg/api/resource"
)

const allNamespacesToken = "__all__"

func (h *Handler) listPVCs(ctx context.Context, req *ListPVCsRequest) (*ListPVCsResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/pvcs", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	if strings.TrimSpace(req.Namespace) == allNamespacesToken {
		return h.listAllPVCs(ctx, principal, start)
	}
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

func (h *Handler) listAllPVCs(
	ctx context.Context,
	principal *authn.Principal,
	start time.Time,
) (*ListPVCsResponse, *apienv.Error) {
	adminResult, adminErr := h.checkAdmin(ctx, principal)
	namespaceAllowed := adminErr == nil && isAdminInOwnNamespace(principal, adminResult)
	if adminErr != nil || !namespaceAllowed {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       adminErr == nil && adminResult.Allowed,
			authorizationKind:  "kubeconfig",
			decision:           "deny",
			denyReason:         adminCapabilityDenyReason(adminErr, adminResult.Reason, namespaceAllowed),
			executionKind:      "management_service_account",
			identityMethod:     "kubeconfig_context+self_subject_review",
			implicitElevation:  true,
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          allNamespacesToken,
			namespaceAllowed:   namespaceAllowed,
			principal:          principal,
			route:              "/pvcs",
		})
		apiErr := apienv.NewError(403, apienv.CodeAdminAccessDenied, "Admin access denied", nil)
		h.observe(ctx, http.MethodGet, "/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	namespaces, err := h.viewers.ListNamespaces(ctx)
	if err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, http.MethodGet, "/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	allowed := allowedAdminNamespaces(namespaces, principal.Namespace)
	allowedNames := make([]string, 0, len(allowed))
	for _, namespace := range allowed {
		allowedNames = append(allowedNames, namespace.Name)
	}
	items, listErr := h.viewers.ListPVCsInNamespaces(ctx, allowedNames)
	if listErr != nil {
		apiErr := apienv.FromError(listErr)
		h.observe(ctx, http.MethodGet, "/pvcs", apiErr.Status, start)
		return nil, apiErr
	}
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       true,
		authorizationKind:  "kubeconfig",
		decision:           "allow",
		executionKind:      "management_service_account",
		identityMethod:     "kubeconfig_context+self_subject_review",
		implicitElevation:  true,
		kubernetesUsername: adminResult.KubernetesUsername,
		mode:               operationModeAdmin,
		namespace:          allNamespacesToken,
		namespaceAllowed:   true,
		principal:          principal,
		route:              "/pvcs",
	})
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

func (h *Handler) getStorageQuota(ctx context.Context, req *StorageQuotaRequest) (*StorageQuotaResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/storage-quota", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, req.Namespace, "/storage-quota", "")
	if apiErr != nil {
		h.observe(ctx, http.MethodGet, "/storage-quota", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanListPVCs(ctx, principal, op.namespace); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "Namespace access denied", nil)
			h.observe(ctx, http.MethodGet, "/storage-quota", apiErr.Status, start)
			return nil, apiErr
		}
	}
	quota, quotaErr := h.storageQuotaForNamespace(ctx, op.namespace, req.SealosAccountAuthorization)
	if quotaErr != nil {
		apiErr := storageQuotaUnavailableError(quotaErr)
		h.observe(ctx, http.MethodGet, "/storage-quota", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/storage-quota", http.StatusOK, start)
	return &StorageQuotaResponse{StorageQuota: storageQuotaResponse(quota)}, nil
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
	if !h.features.PVCCreation.Enabled {
		apiErr := apienv.NewError(403, apienv.CodePVCCreateForbidden, "PVC creation is disabled", map[string]any{
			"reason": "feature_disabled",
		})
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       op.mode == operationModeAdmin,
			decision:           "deny",
			denyReason:         "feature_disabled",
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
	if apiErr := h.requireStorageQuota(ctx, op.namespace, req.SealosAccountAuthorization, requestedCapacityBytes(req.Capacity, req.CapacityBytes)); apiErr != nil {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       op.mode == operationModeAdmin,
			decision:           "deny",
			denyReason:         "storage_quota",
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
		h.observe(ctx, http.MethodPost, "/pvcs", apiErr.Status, start)
		return nil, apiErr
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

func (h *Handler) getPVCYAML(
	ctx context.Context,
	namespace string,
	name string,
	req *AuthenticatedRequest,
) (*PVCYAMLResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/pvcs/:namespace/:name/yaml", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, namespace, "/pvcs/:namespace/:name/yaml", name)
	if apiErr != nil {
		h.observe(ctx, http.MethodGet, "/pvcs/:namespace/:name/yaml", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanGetPVC(ctx, principal, op.namespace, name); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
			h.observe(ctx, http.MethodGet, "/pvcs/:namespace/:name/yaml", apiErr.Status, start)
			return nil, apiErr
		}
	}
	result, getErr := op.kubeService.GetPVCYAML(ctx, op.namespace, name)
	if getErr != nil {
		apiErr := apienv.FromError(getErr)
		h.observe(ctx, http.MethodGet, "/pvcs/:namespace/:name/yaml", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/pvcs/:namespace/:name/yaml", http.StatusOK, start)
	return &PVCYAMLResponse{PVCYAML: result}, nil
}

func (h *Handler) updatePVC(
	ctx context.Context,
	namespace string,
	name string,
	req *PVCYAMLRequest,
) (*PVCResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodPut, "/pvcs/:namespace/:name", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, namespace, "/pvcs/:namespace/:name", name)
	if apiErr != nil {
		h.observe(ctx, http.MethodPut, "/pvcs/:namespace/:name", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanUpdatePVC(ctx, principal, op.namespace, name); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCExpandForbidden, "PVC update access denied", nil)
			h.observe(ctx, http.MethodPut, "/pvcs/:namespace/:name", apiErr.Status, start)
			return nil, apiErr
		}
	}
	pvc, updateErr := op.kubeService.UpdatePVC(ctx, op.namespace, name, req.YAML)
	if updateErr != nil {
		apiErr := apienv.FromError(updateErr)
		h.observe(ctx, http.MethodPut, "/pvcs/:namespace/:name", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPut, "/pvcs/:namespace/:name", http.StatusOK, start)
	return &PVCResponse{PVC: pvc}, nil
}

func (h *Handler) describePVC(
	ctx context.Context,
	namespace string,
	name string,
	req *AuthenticatedRequest,
) (*PVCDescribeResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/pvcs/:namespace/:name/describe", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	op, apiErr := h.resolvePVCOperationContext(ctx, principal, namespace, "/pvcs/:namespace/:name/describe", name)
	if apiErr != nil {
		h.observe(ctx, http.MethodGet, "/pvcs/:namespace/:name/describe", apiErr.Status, start)
		return nil, apiErr
	}
	if op.mode == operationModeUser {
		if err := h.authz.CanGetPVC(ctx, principal, op.namespace, name); err != nil {
			apiErr := apienv.NewError(403, apienv.CodePVCAccessDenied, "PVC access denied", nil)
			h.observe(ctx, http.MethodGet, "/pvcs/:namespace/:name/describe", apiErr.Status, start)
			return nil, apiErr
		}
	}
	result, describeErr := op.kubeService.DescribePVC(ctx, op.namespace, name)
	if describeErr != nil {
		apiErr := apienv.FromError(describeErr)
		h.observe(ctx, http.MethodGet, "/pvcs/:namespace/:name/describe", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/pvcs/:namespace/:name/describe", http.StatusOK, start)
	return &PVCDescribeResponse{PVCDescribe: result}, nil
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
	currentPVCs, listErr := op.kubeService.ListPVCs(ctx, op.namespace)
	if listErr != nil {
		apiErr := apienv.FromError(listErr)
		h.observe(ctx, http.MethodPost, "/pvcs/:namespace/:name/expand", apiErr.Status, start)
		return nil, apiErr
	}
	requiredBytes := requestedCapacityBytes(req.Capacity, req.CapacityBytes) - currentPVCCapacityBytes(currentPVCs, name)
	if requiredBytes < 0 {
		requiredBytes = 0
	}
	if apiErr := h.requireStorageQuota(ctx, op.namespace, req.SealosAccountAuthorization, requiredBytes); apiErr != nil {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       op.mode == operationModeAdmin,
			decision:           "deny",
			denyReason:         "storage_quota",
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
		h.observe(ctx, http.MethodPost, "/pvcs/:namespace/:name/expand", apiErr.Status, start)
		return nil, apiErr
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

func (h *Handler) requireStorageQuota(
	ctx context.Context,
	namespace string,
	authorization string,
	requiredBytes int64,
) *apienv.Error {
	if requiredBytes <= 0 {
		return nil
	}
	quota, err := h.storageQuotaForNamespace(ctx, namespace, authorization)
	if err != nil {
		return storageQuotaUnavailableError(err)
	}
	if requiredBytes > quota.AvailableBytes {
		return apienv.NewError(403, apienv.CodePVCQuotaExceeded, "Storage quota exceeded", map[string]any{
			"available_bytes":    quota.AvailableBytes,
			"available_quantity": quota.AvailableQuantity,
			"requested_bytes":    requiredBytes,
			"requested_quantity": accountquota.BinaryQuantity(requiredBytes),
		})
	}
	return nil
}

func (h *Handler) storageQuotaForNamespace(
	ctx context.Context,
	namespace string,
	authorization string,
) (accountquota.StorageQuota, error) {
	if !isUserNamespace(namespace) {
		return fixedSystemStorageQuota(h.features.StorageQuota.SystemQuota)
	}
	return h.storageQuota.StorageQuota(ctx, namespace, authorization)
}

func isUserNamespace(namespace string) bool {
	return strings.HasPrefix(strings.TrimSpace(namespace), "ns-")
}

func fixedSystemStorageQuota(systemQuota string) (accountquota.StorageQuota, error) {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(systemQuota))
	if err != nil {
		return accountquota.StorageQuota{}, err
	}
	return accountquota.StorageQuota{
		AvailableBytes:    quantity.Value(),
		AvailableQuantity: quantity.String(),
		LimitBytes:        quantity.Value(),
		LimitQuantity:     quantity.String(),
		UsedQuantity:      "0",
	}, nil
}

func storageQuotaUnavailableError(err error) *apienv.Error {
	return apienv.NewError(503, apienv.CodePVCQuotaUnavailable, "Storage quota is unavailable", map[string]any{
		"reason": err.Error(),
	})
}

func storageQuotaResponse(quota accountquota.StorageQuota) StorageQuota {
	return StorageQuota{
		AvailableBytes:    quota.AvailableBytes,
		AvailableQuantity: quota.AvailableQuantity,
		LimitBytes:        quota.LimitBytes,
		LimitQuantity:     quota.LimitQuantity,
		UsedBytes:         quota.UsedBytes,
		UsedQuantity:      quota.UsedQuantity,
	}
}

func currentPVCCapacityBytes(pvcs []domain.PVC, name string) int64 {
	for _, pvc := range pvcs {
		if pvc.Name == name {
			return pvc.CapacityBytes
		}
	}
	return 0
}

func requestedCapacityBytes(capacity string, capacityBytes int64) int64 {
	if capacityBytes > 0 {
		return capacityBytes
	}
	if strings.TrimSpace(capacity) == "" {
		return 0
	}
	quantity, err := resource.ParseQuantity(strings.TrimSpace(capacity))
	if err != nil {
		return 0
	}
	return quantity.Value()
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
