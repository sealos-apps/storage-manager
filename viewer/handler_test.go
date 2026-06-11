package viewer

import (
	"context"
	"errors"
	"sync"

	"github.com/nixieboluo/sealos-storage-manager/internal/accountquota"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type fakeViewerService struct {
	pvcs           []domain.PVC
	pvc            *domain.PVC
	namespaces     []corev1.Namespace
	storageClasses []domain.StorageClass
	created        *domain.ViewerSession
	createInput    *session.CreateViewerSessionInput
	deleteInput    *session.DeletePVCInput
	expandInput    *session.ExpandPVCInput
	pvcInput       *session.CreatePVCInput
	token          *domain.ViewerToken
	tokenInput     *viewerSessionCall
	heartbeat      *domain.Heartbeat
	heartbeatInput *viewerSessionCall
	closed         *domain.ViewerSession
	closeInput     *viewerSessionCall
	podSession     *domain.PodSession
	podSessionErr  error
}

type viewerSessionCall struct {
	id     string
	userID string
}

type fakeStorageClassService struct {
	items    []domain.StorageClass
	yaml     *session.StorageClassYAML
	describe *session.StorageClassDescribe
	item     *domain.StorageClass
}

type fakeStorageQuotaService struct {
	err      error
	input    *storageQuotaCall
	calls    *int
	response accountquota.StorageQuota
}

type storageQuotaCall struct {
	authorization string
	namespace     string
}

const testKubeconfig = `apiVersion: v1
kind: Config
current-context: dev
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
users:
- name: u
  user:
    token: test-token
contexts:
- name: dev
  context:
    cluster: c
    user: u
    namespace: ns
`

const testUserNamespaceKubeconfig = `apiVersion: v1
kind: Config
current-context: dev
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
users:
- name: u
  user:
    token: test-token
contexts:
- name: dev
  context:
    cluster: c
    user: u
    namespace: ns-admin
`

const testOtherUserNamespaceKubeconfig = `apiVersion: v1
kind: Config
current-context: dev
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
users:
- name: u
  user:
    token: test-token
contexts:
- name: dev
  context:
    cluster: c
    user: u
    namespace: ns-rm68q0bp
`

const testSystemNamespaceKubeconfig = `apiVersion: v1
kind: Config
current-context: dev
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
users:
- name: u
  user:
    token: test-token
contexts:
- name: dev
  context:
    cluster: c
    user: u
    namespace: kube-system
`

func (f *fakeViewerService) ListPVCs(_ context.Context, namespace string) ([]domain.PVC, error) {
	if len(f.pvcs) > 0 {
		for i := range f.pvcs {
			f.pvcs[i].Namespace = namespace
		}
	}
	return f.pvcs, nil
}

func (f *fakeViewerService) ListNamespaces(_ context.Context) ([]corev1.Namespace, error) {
	return f.namespaces, nil
}

func (f *fakeViewerService) CreatePVC(_ context.Context, input session.CreatePVCInput) (*domain.PVC, error) {
	if f.pvcInput != nil {
		*f.pvcInput = input
	}
	return f.pvc, nil
}

func (f *fakeViewerService) DeletePVC(_ context.Context, input session.DeletePVCInput) (*domain.PVC, error) {
	if f.deleteInput != nil {
		*f.deleteInput = input
	}
	return f.pvc, nil
}

func (f *fakeViewerService) ExpandPVC(_ context.Context, input session.ExpandPVCInput) (*domain.PVC, error) {
	if f.expandInput != nil {
		*f.expandInput = input
	}
	return f.pvc, nil
}

func (f *fakeViewerService) ListStorageClasses(_ context.Context) ([]domain.StorageClass, error) {
	return f.storageClasses, nil
}

func (f *fakeViewerService) CreateViewerSession(
	_ context.Context,
	input session.CreateViewerSessionInput,
) (*domain.ViewerSession, error) {
	if f.createInput != nil {
		*f.createInput = input
	}
	return f.created, nil
}

func (f *fakeViewerService) GetViewerSession(
	_ context.Context,
	_ string,
	_ string,
) (*domain.ViewerSession, error) {
	return f.created, nil
}

func (f *fakeViewerService) IssueToken(_ context.Context, id string, userID string) (*domain.ViewerToken, error) {
	if f.tokenInput != nil {
		*f.tokenInput = viewerSessionCall{id: id, userID: userID}
	}
	return f.token, nil
}

func (f *fakeViewerService) HeartbeatForUser(_ context.Context, id string, userID string) (*domain.Heartbeat, error) {
	if f.heartbeatInput != nil {
		*f.heartbeatInput = viewerSessionCall{id: id, userID: userID}
	}
	return f.heartbeat, nil
}

func (f *fakeViewerService) CloseViewerSessionForUser(id string, userID string) (*domain.ViewerSession, error) {
	if f.closeInput != nil {
		*f.closeInput = viewerSessionCall{id: id, userID: userID}
	}
	return f.closed, nil
}

func (f *fakeViewerService) GetPodSession(_ string) (*domain.PodSession, error) {
	if f.podSessionErr != nil {
		return nil, f.podSessionErr
	}
	return f.podSession, nil
}

func (f fakeStorageClassService) ListStorageClasses(_ context.Context, includeHidden bool) ([]domain.StorageClass, error) {
	if !includeHidden {
		return nil, errors.New("admin list must include hidden storageclasses")
	}
	return f.items, nil
}

func (f fakeStorageClassService) GetStorageClassYAML(_ context.Context, _ string) (*session.StorageClassYAML, error) {
	return f.yaml, nil
}

func (f fakeStorageClassService) CreateStorageClass(_ context.Context, _ string) (*domain.StorageClass, error) {
	return f.item, nil
}

func (f fakeStorageClassService) UpdateStorageClass(_ context.Context, _ string, _ string) (*domain.StorageClass, error) {
	return f.item, nil
}

func (f fakeStorageClassService) DeleteStorageClass(_ context.Context, _ string) (*domain.StorageClass, error) {
	return f.item, nil
}

func (f fakeStorageClassService) DescribeStorageClass(_ context.Context, _ string) (*session.StorageClassDescribe, error) {
	return f.describe, nil
}

func (f fakeStorageQuotaService) StorageQuota(
	_ context.Context,
	namespace string,
	authorization string,
) (accountquota.StorageQuota, error) {
	if f.calls != nil {
		(*f.calls)++
	}
	if f.input != nil {
		*f.input = storageQuotaCall{
			authorization: authorization,
			namespace:     namespace,
		}
	}
	if f.err != nil {
		return accountquota.StorageQuota{}, f.err
	}
	return f.response, nil
}

type fakePodService struct {
	closed *domain.PodSession
}

func (f fakePodService) ClosePodSession(_ context.Context, _ string) (*domain.PodSession, error) {
	return f.closed, nil
}

type fakeAuthService struct {
	result domain.FileBrowserHookVerification
}

func (f fakeAuthService) VerifyHook(_ session.HookVerifyInput) domain.FileBrowserHookVerification {
	return f.result
}

func testDisabledFileManagement() config.FeatureConfig {
	cfg := config.Default()
	cfg.Viewer.FileManagement.Enabled = false
	return cfg.Features()
}

type allowAuthorizer struct{}

var clientsetFactoryMu sync.Mutex

func (allowAuthorizer) CanListPVCs(_ context.Context, _ *authn.Principal, _ string) error {
	return nil
}

func (allowAuthorizer) CanGetPVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return nil
}

func (allowAuthorizer) CanCreatePVC(_ context.Context, _ *authn.Principal, _ string) error {
	return nil
}

func (allowAuthorizer) CanDeletePVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return nil
}

func (allowAuthorizer) CanUpdatePVC(
	_ context.Context,
	_ *authn.Principal,
	_ string,
	_ string,
) error {
	return nil
}

func (allowAuthorizer) CanListStorageClasses(_ context.Context, _ *authn.Principal) error {
	return nil
}

type allowAdminAuthorizer struct{}

func (allowAdminAuthorizer) CanAdmin(_ context.Context, _ *authn.Principal) (AdminAuthorizationResult, error) {
	return AdminAuthorizationResult{
		Allowed:            true,
		AllowedNamespace:   "ns-admin",
		KubernetesUsername: "system:serviceaccount:user-system:admin",
		Reason:             "allowed",
	}, nil
}

func (allowAdminAuthorizer) CanManageStorageClasses(_ context.Context, _ *authn.Principal) error {
	return nil
}

type denyTestAdminAuthorizer struct{}

func (denyTestAdminAuthorizer) CanAdmin(_ context.Context, _ *authn.Principal) (AdminAuthorizationResult, error) {
	return AdminAuthorizationResult{Reason: "not_admin"}, errors.New("denied")
}

func (denyTestAdminAuthorizer) CanManageStorageClasses(_ context.Context, _ *authn.Principal) error {
	return errors.New("denied")
}

func namespaceWithUID(name string, uid string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(uid),
		},
	}
}

func pvcWithUID(namespace string, name string, uid string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			UID:       types.UID(uid),
		},
	}
}

func testObservability() config.ObservabilityConfig {
	cfg := config.Default().Observability
	cfg.Logs.Exporter = "discard"
	cfg.Logs.Level = "error"
	return cfg
}
