package viewer

import (
	"context"
	"math"

	"github.com/nixieboluo/sealos-storage-manager/internal/accountquota"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	corev1 "k8s.io/api/core/v1"
)

type unavailableViewerService struct{}

func (unavailableViewerService) ListNamespaces(_ context.Context) ([]corev1.Namespace, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) ListPVCs(_ context.Context, _ string) ([]domain.PVC, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) ListPVCsInNamespaces(_ context.Context, _ []string) ([]domain.PVC, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) CreatePVC(_ context.Context, _ session.CreatePVCInput) (*domain.PVC, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) DeletePVC(_ context.Context, _ session.DeletePVCInput) (*domain.PVC, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) ExpandPVC(_ context.Context, _ session.ExpandPVCInput) (*domain.PVC, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) ListStorageClasses(_ context.Context) ([]domain.StorageClass, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) CreateViewerSession(
	_ context.Context,
	_ session.CreateViewerSessionInput,
) (*domain.ViewerSession, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) GetViewerSession(
	_ context.Context,
	_ string,
	_ string,
) (*domain.ViewerSession, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) IssueToken(_ context.Context, _ string, _ string) (*domain.ViewerToken, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) HeartbeatForUser(_ context.Context, _ string, _ string) (*domain.Heartbeat, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) CloseViewerSessionForUser(_ string, _ string) (*domain.ViewerSession, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableViewerService) GetPodSession(_ string) (*domain.PodSession, error) {
	return nil, errRuntimeUnavailable
}

type unavailableStorageClassService struct{}

func (unavailableStorageClassService) ListStorageClasses(_ context.Context, _ bool) ([]domain.StorageClass, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableStorageClassService) GetStorageClassYAML(
	_ context.Context,
	_ string,
) (*session.StorageClassYAML, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableStorageClassService) CreateStorageClass(_ context.Context, _ string) (*domain.StorageClass, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableStorageClassService) UpdateStorageClass(
	_ context.Context,
	_ string,
	_ string,
) (*domain.StorageClass, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableStorageClassService) DeleteStorageClass(_ context.Context, _ string) (*domain.StorageClass, error) {
	return nil, errRuntimeUnavailable
}

func (unavailableStorageClassService) DescribeStorageClass(
	_ context.Context,
	_ string,
) (*session.StorageClassDescribe, error) {
	return nil, errRuntimeUnavailable
}

type unavailablePodService struct{}

func (unavailablePodService) ClosePodSession(_ context.Context, _ string) (*domain.PodSession, error) {
	return nil, errRuntimeUnavailable
}

type unavailableAuthService struct{}

func (unavailableAuthService) VerifyHook(_ session.HookVerifyInput) domain.FileBrowserHookVerification {
	return domain.FileBrowserHookVerification{
		Allow:  false,
		Reason: errRuntimeUnavailable.Error(),
		Scope:  "/",
	}
}

type disabledStorageQuotaService struct{}

func (disabledStorageQuotaService) StorageQuota(
	context.Context,
	string,
	string,
) (accountquota.StorageQuota, error) {
	return disabledStorageQuotaService{}.quota(), nil
}

func (disabledStorageQuotaService) quota() accountquota.StorageQuota {
	return accountquota.StorageQuota{
		AvailableBytes:    math.MaxInt64,
		AvailableQuantity: accountquota.BinaryQuantity(math.MaxInt64),
		LimitBytes:        math.MaxInt64,
		LimitQuantity:     accountquota.BinaryQuantity(math.MaxInt64),
		UsedQuantity:      "0",
	}
}

type denyAuthorizer struct{}

func (denyAuthorizer) CanListPVCs(_ context.Context, _ *authn.Principal, _ string) error {
	return errRuntimeUnavailable
}

func (denyAuthorizer) CanGetPVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return errRuntimeUnavailable
}

func (denyAuthorizer) CanCreatePVC(_ context.Context, _ *authn.Principal, _ string) error {
	return errRuntimeUnavailable
}

func (denyAuthorizer) CanDeletePVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return errRuntimeUnavailable
}

func (denyAuthorizer) CanUpdatePVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return errRuntimeUnavailable
}

func (denyAuthorizer) CanListStorageClasses(_ context.Context, _ *authn.Principal) error {
	return errRuntimeUnavailable
}
