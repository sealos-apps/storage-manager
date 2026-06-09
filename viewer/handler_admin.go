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

func (h *Handler) adminCapabilities(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*AdminCapabilitiesResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/admin/capabilities", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	adminResult, adminErr := h.checkAdmin(ctx, principal)
	canManage := adminErr == nil && adminResult.Allowed
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       canManage,
		authorizationKind:  "kubeconfig",
		decision:           allowDeny(canManage),
		denyReason:         denyReason(adminErr, adminResult.Reason),
		executionKind:      "management_service_account",
		identityMethod:     "kubeconfig_context+self_subject_review",
		kubernetesUsername: adminResult.KubernetesUsername,
		mode:               operationModeAdmin,
		namespace:          principal.Namespace,
		namespaceAllowed:   true,
		principal:          principal,
		route:              "/admin/capabilities",
	})
	h.observe(ctx, http.MethodGet, "/admin/capabilities", http.StatusOK, start)
	return &AdminCapabilitiesResponse{
		AdminCapabilities: AdminCapabilitySet{
			CanManagePVCs:           canManage,
			CanManageStorageClasses: canManage,
			FileManagementEnabled:   h.features.FileManagement.Enabled,
		},
	}, nil
}

func (h *Handler) adminListNamespaces(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListNamespacesResponse, *apienv.Error) {
	start := time.Now()
	principal, err := h.authenticateRequest(req)
	if err != nil {
		h.observe(ctx, http.MethodGet, "/admin/namespaces", err.Status, start)
		return nil, err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	adminResult, adminErr := h.checkAdmin(ctx, principal)
	if adminErr != nil {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       false,
			authorizationKind:  "kubeconfig",
			decision:           "deny",
			denyReason:         denyReason(adminErr, adminResult.Reason),
			executionKind:      "management_service_account",
			identityMethod:     "kubeconfig_context+self_subject_review",
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          principal.Namespace,
			namespaceAllowed:   false,
			principal:          principal,
			route:              "/admin/namespaces",
		})
		apiErr := apienv.NewError(403, apienv.CodeAdminAccessDenied, "Admin access denied", nil)
		h.observe(ctx, http.MethodGet, "/admin/namespaces", apiErr.Status, start)
		return nil, apiErr
	}
	namespaces, listErr := h.viewers.ListNamespaces(ctx)
	if listErr != nil {
		apiErr := apienv.FromError(listErr)
		h.observe(ctx, http.MethodGet, "/admin/namespaces", apiErr.Status, start)
		return nil, apiErr
	}
	items := allowedAdminNamespaces(namespaces, principal.Namespace)
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       true,
		authorizationKind:  "kubeconfig",
		decision:           "allow",
		executionKind:      "management_service_account",
		identityMethod:     "kubeconfig_context+self_subject_review",
		kubernetesUsername: adminResult.KubernetesUsername,
		mode:               operationModeAdmin,
		namespace:          principal.Namespace,
		namespaceAllowed:   true,
		principal:          principal,
		route:              "/admin/namespaces",
	})
	h.observe(ctx, http.MethodGet, "/admin/namespaces", http.StatusOK, start)
	return &ListNamespacesResponse{NamespaceList: NamespaceList{Items: items}}, nil
}

func (h *Handler) adminListStorageClasses(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListStorageClassesResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodGet, "/admin/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	items, listErr := h.storageClasses.ListStorageClasses(ctx, true)
	if listErr != nil {
		apiErr := apienv.FromError(listErr)
		h.observe(ctx, http.MethodGet, "/admin/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	if items == nil {
		items = []domain.StorageClass{}
	}
	h.observe(ctx, http.MethodGet, "/admin/storage-classes", http.StatusOK, start)
	return &ListStorageClassesResponse{StorageClassList: StorageClassList{Items: items}}, nil
}

func (h *Handler) adminGetStorageClassYAML(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassYAMLResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodGet, "/admin/storage-classes/:name/yaml", apiErr.Status, start)
		return nil, apiErr
	}
	result, getErr := h.storageClasses.GetStorageClassYAML(ctx, name)
	if getErr != nil {
		apiErr := apienv.FromError(getErr)
		h.observe(ctx, http.MethodGet, "/admin/storage-classes/:name/yaml", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/admin/storage-classes/:name/yaml", http.StatusOK, start)
	return &StorageClassYAMLResponse{StorageClassYAML: result}, nil
}

func (h *Handler) adminCreateStorageClass(
	ctx context.Context,
	req *StorageClassYAMLRequest,
) (*StorageClassResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodPost, "/admin/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	item, createErr := h.storageClasses.CreateStorageClass(ctx, req.YAML)
	if createErr != nil {
		apiErr := apienv.FromError(createErr)
		h.observe(ctx, http.MethodPost, "/admin/storage-classes", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPost, "/admin/storage-classes", http.StatusCreated, start)
	return &StorageClassResponse{StorageClass: item}, nil
}

func (h *Handler) adminUpdateStorageClass(
	ctx context.Context,
	name string,
	req *StorageClassYAMLRequest,
) (*StorageClassResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodPut, "/admin/storage-classes/:name", apiErr.Status, start)
		return nil, apiErr
	}
	item, updateErr := h.storageClasses.UpdateStorageClass(ctx, name, req.YAML)
	if updateErr != nil {
		apiErr := apienv.FromError(updateErr)
		h.observe(ctx, http.MethodPut, "/admin/storage-classes/:name", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPut, "/admin/storage-classes/:name", http.StatusOK, start)
	return &StorageClassResponse{StorageClass: item}, nil
}

func (h *Handler) adminUpdateStorageClassPolicy(
	ctx context.Context,
	name string,
	req *StorageClassPolicyRequest,
) (*StorageClassResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodPut, "/admin/storage-classes/:name/policy", apiErr.Status, start)
		return nil, apiErr
	}
	item, updateErr := h.storageClasses.UpdateStorageClassPolicy(
		ctx,
		name,
		session.StorageClassPolicyInput{
			AllowedAccessModes: req.AllowedAccessModes,
			VisibleInCreate:    req.VisibleInCreate,
		},
	)
	if updateErr != nil {
		apiErr := apienv.FromError(updateErr)
		h.observe(ctx, http.MethodPut, "/admin/storage-classes/:name/policy", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodPut, "/admin/storage-classes/:name/policy", http.StatusOK, start)
	return &StorageClassResponse{StorageClass: item}, nil
}

func (h *Handler) adminDeleteStorageClass(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodDelete, "/admin/storage-classes/:name", apiErr.Status, start)
		return nil, apiErr
	}
	item, deleteErr := h.storageClasses.DeleteStorageClass(ctx, name)
	if deleteErr != nil {
		apiErr := apienv.FromError(deleteErr)
		h.observe(ctx, http.MethodDelete, "/admin/storage-classes/:name", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodDelete, "/admin/storage-classes/:name", http.StatusOK, start)
	return &StorageClassResponse{StorageClass: item}, nil
}

func (h *Handler) adminDescribeStorageClass(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassDescribeResponse, *apienv.Error) {
	start := time.Now()
	if apiErr := h.authorizeStorageClassAdmin(ctx, req); apiErr != nil {
		h.observe(ctx, http.MethodGet, "/admin/storage-classes/:name/describe", apiErr.Status, start)
		return nil, apiErr
	}
	result, describeErr := h.storageClasses.DescribeStorageClass(ctx, name)
	if describeErr != nil {
		apiErr := apienv.FromError(describeErr)
		h.observe(ctx, http.MethodGet, "/admin/storage-classes/:name/describe", apiErr.Status, start)
		return nil, apiErr
	}
	h.observe(ctx, http.MethodGet, "/admin/storage-classes/:name/describe", http.StatusOK, start)
	return &StorageClassDescribeResponse{StorageClassDescribe: result}, nil
}

func (h *Handler) authorizeStorageClassAdmin(
	ctx context.Context,
	req interface{ authorizationHeader() string },
) *apienv.Error {
	principal, err := h.authenticateRequest(req)
	if err != nil {
		return err
	}
	ctx = authn.WithPrincipal(ctx, principal)
	adminResult, adminErr := h.checkAdmin(ctx, principal)
	if adminErr != nil {
		h.recordAudit(ctx, auditDecision{
			adminAllowed:       false,
			decision:           "deny",
			denyReason:         denyReason(adminErr, adminResult.Reason),
			identityMethod:     "kubeconfig_context+self_subject_review",
			kubernetesUsername: adminResult.KubernetesUsername,
			mode:               operationModeAdmin,
			namespace:          principal.Namespace,
			namespaceAllowed:   true,
			principal:          principal,
			route:              "/admin/storage-classes",
		})
		return apienv.NewError(403, apienv.CodeAdminAccessDenied, "Admin access denied", nil)
	}
	h.recordAudit(ctx, auditDecision{
		adminAllowed:       true,
		decision:           "allow",
		identityMethod:     "kubeconfig_context+self_subject_review",
		kubernetesUsername: adminResult.KubernetesUsername,
		mode:               operationModeAdmin,
		namespace:          principal.Namespace,
		namespaceAllowed:   true,
		principal:          principal,
		route:              "/admin/storage-classes",
	})
	return nil
}
