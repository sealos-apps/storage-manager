package viewer

import "context"

func (h *Handler) ListPVCsData(ctx context.Context, req *ListPVCsRequest) (*ListPVCsResponse, error) {
	response, apiErr := h.listPVCs(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) GetContextData(ctx context.Context, req *AuthenticatedRequest) (*ContextResponse, error) {
	response, apiErr := h.getContext(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) GetStorageQuotaData(ctx context.Context, req *StorageQuotaRequest) (*StorageQuotaResponse, error) {
	response, apiErr := h.getStorageQuota(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) CreatePVCData(ctx context.Context, req *CreatePVCRequest) (*PVCResponse, error) {
	response, apiErr := h.createPVC(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) DeletePVCData(
	ctx context.Context,
	namespace string,
	name string,
	req *AuthenticatedRequest,
) (*PVCResponse, error) {
	response, apiErr := h.deletePVC(ctx, namespace, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) GetPVCYAMLData(
	ctx context.Context,
	namespace string,
	name string,
	req *AuthenticatedRequest,
) (*PVCYAMLResponse, error) {
	response, apiErr := h.getPVCYAML(ctx, namespace, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) UpdatePVCData(
	ctx context.Context,
	namespace string,
	name string,
	req *PVCYAMLRequest,
) (*PVCResponse, error) {
	response, apiErr := h.updatePVC(ctx, namespace, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) DescribePVCData(
	ctx context.Context,
	namespace string,
	name string,
	req *AuthenticatedRequest,
) (*PVCDescribeResponse, error) {
	response, apiErr := h.describePVC(ctx, namespace, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) ExpandPVCData(
	ctx context.Context,
	namespace string,
	name string,
	req *ExpandPVCRequest,
) (*PVCResponse, error) {
	response, apiErr := h.expandPVC(ctx, namespace, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) ListStorageClassesData(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListStorageClassesResponse, error) {
	response, apiErr := h.listStorageClasses(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminCapabilitiesData(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*AdminCapabilitiesResponse, error) {
	response, apiErr := h.adminCapabilities(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminListNamespacesData(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListNamespacesResponse, error) {
	response, apiErr := h.adminListNamespaces(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminListStorageClassesData(
	ctx context.Context,
	req *AuthenticatedRequest,
) (*ListStorageClassesResponse, error) {
	response, apiErr := h.adminListStorageClasses(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminGetStorageClassYAMLData(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassYAMLResponse, error) {
	response, apiErr := h.adminGetStorageClassYAML(ctx, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminCreateStorageClassData(
	ctx context.Context,
	req *StorageClassYAMLRequest,
) (*StorageClassResponse, error) {
	response, apiErr := h.adminCreateStorageClass(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminUpdateStorageClassData(
	ctx context.Context,
	name string,
	req *StorageClassYAMLRequest,
) (*StorageClassResponse, error) {
	response, apiErr := h.adminUpdateStorageClass(ctx, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminUpdateStorageClassMetadataData(
	ctx context.Context,
	name string,
	req *StorageClassMetadataRequest,
) (*StorageClassResponse, error) {
	response, apiErr := h.adminUpdateStorageClassMetadata(ctx, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminDeleteStorageClassData(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassResponse, error) {
	response, apiErr := h.adminDeleteStorageClass(ctx, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) AdminDescribeStorageClassData(
	ctx context.Context,
	name string,
	req *AuthenticatedRequest,
) (*StorageClassDescribeResponse, error) {
	response, apiErr := h.adminDescribeStorageClass(ctx, name, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) CreateViewerSessionData(
	ctx context.Context,
	req *CreateViewerSessionRequest,
) (*ViewerSessionResponse, error) {
	response, apiErr := h.createViewerSession(ctx, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) GetViewerSessionData(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, error) {
	response, apiErr := h.getViewerSession(ctx, viewerSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) IssueTokenData(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerTokenResponse, error) {
	response, apiErr := h.issueToken(ctx, viewerSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) HeartbeatData(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*HeartbeatResponse, error) {
	response, apiErr := h.heartbeat(ctx, viewerSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) CloseViewerSessionData(
	ctx context.Context,
	viewerSessionID string,
	req *AuthenticatedRequest,
) (*ViewerSessionResponse, error) {
	response, apiErr := h.closeViewerSession(ctx, viewerSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) ClosePodSessionData(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, error) {
	response, apiErr := h.closePodSession(ctx, podSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) GetPodSessionData(
	ctx context.Context,
	podSessionID string,
	req *AuthenticatedRequest,
) (*PodSessionResponse, error) {
	response, apiErr := h.getPodSession(ctx, podSessionID, req)
	return response, toEncoreError(apiErr)
}

func (h *Handler) VerifyFileBrowserHookData(
	ctx context.Context,
	req *VerifyFileBrowserHookRequest,
) (*FileBrowserHookVerificationResponse, error) {
	response, apiErr := h.verifyFileBrowserHook(ctx, req)
	return response, toEncoreError(apiErr)
}
