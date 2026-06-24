package viewer

import (
	"context"

	"github.com/nixieboluo/sealos-storage-manager/internal/accountquota"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var kubernetesClientsetForConfig = func(c *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(c)
}

type viewerService interface {
	ListNamespaces(ctx context.Context) ([]corev1.Namespace, error)
	ListPVCs(ctx context.Context, namespace string) ([]domain.PVC, error)
	ListPVCsInNamespaces(ctx context.Context, namespaces []string) ([]domain.PVC, error)
	CreatePVC(ctx context.Context, input session.CreatePVCInput) (*domain.PVC, error)
	DeletePVC(ctx context.Context, input session.DeletePVCInput) (*domain.PVC, error)
	GetPVCYAML(ctx context.Context, namespace string, name string) (*session.PVCYAML, error)
	UpdatePVC(ctx context.Context, namespace string, name string, body string) (*domain.PVC, error)
	DescribePVC(ctx context.Context, namespace string, name string) (*session.PVCDescribe, error)
	ExpandPVC(ctx context.Context, input session.ExpandPVCInput) (*domain.PVC, error)
	ListStorageClasses(ctx context.Context) ([]domain.StorageClass, error)
	CreateViewerSession(ctx context.Context, input session.CreateViewerSessionInput) (*domain.ViewerSession, error)
	GetViewerSession(ctx context.Context, id string, userID string) (*domain.ViewerSession, error)
	IssueToken(ctx context.Context, id string, userID string) (*domain.ViewerToken, error)
	HeartbeatForUser(ctx context.Context, id string, userID string) (*domain.Heartbeat, error)
	CloseViewerSessionForUser(id string, userID string) (*domain.ViewerSession, error)
	GetPodSession(id string) (*domain.PodSession, error)
}

type storageClassService interface {
	ListStorageClasses(ctx context.Context, includeHidden bool) ([]domain.StorageClass, error)
	GetStorageClassYAML(ctx context.Context, name string) (*session.StorageClassYAML, error)
	CreateStorageClass(ctx context.Context, body string) (*domain.StorageClass, error)
	UpdateStorageClass(ctx context.Context, name string, body string) (*domain.StorageClass, error)
	UpdateStorageClassMetadata(ctx context.Context, name string, input session.StorageClassMetadataInput) (*domain.StorageClass, error)
	DeleteStorageClass(ctx context.Context, name string) (*domain.StorageClass, error)
	DescribeStorageClass(ctx context.Context, name string) (*session.StorageClassDescribe, error)
}

type podService interface {
	ClosePodSession(ctx context.Context, podSessionID string) (*domain.PodSession, error)
}

type authService interface {
	VerifyHook(input session.HookVerifyInput) domain.FileBrowserHookVerification
}

type storageQuotaService interface {
	StorageQuota(ctx context.Context, namespace string, authorization string) (accountquota.StorageQuota, error)
}

type authorizer interface {
	CanListPVCs(ctx context.Context, principal *authn.Principal, namespace string) error
	CanGetPVC(ctx context.Context, principal *authn.Principal, namespace string, name string) error
	CanCreatePVC(ctx context.Context, principal *authn.Principal, namespace string) error
	CanDeletePVC(ctx context.Context, principal *authn.Principal, namespace string, name string) error
	CanUpdatePVC(ctx context.Context, principal *authn.Principal, namespace string, name string) error
	CanListStorageClasses(ctx context.Context, principal *authn.Principal) error
}

type adminAuthorizer interface {
	CanAdmin(ctx context.Context, principal *authn.Principal) (AdminAuthorizationResult, error)
	CanManageStorageClasses(ctx context.Context, principal *authn.Principal) error
}

type Handler struct {
	viewers              viewerService
	storageClasses       storageClassService
	pods                 podService
	auth                 authService
	storageQuota         storageQuotaService
	recorder             *observability.Recorder
	authz                authorizer
	adminAuthz           adminAuthorizer
	managementRESTConfig *rest.Config
	debug                config.DebugConfig
	features             config.FeatureConfig
}

type HandlerOption func(*Handler)

type operationMode string

const (
	operationModeUser  operationMode = "user"
	operationModeAdmin operationMode = "admin"
)

type operationContext struct {
	admin             AdminAuthorizationResult
	adminContext      bool
	implicitElevation bool
	kubeService       viewerService
	mode              operationMode
	namespace         string
	namespaceAllowed  bool
	principal         *authn.Principal
}

type auditDecision struct {
	adminAllowed       bool
	authorizationKind  string
	decision           string
	denyReason         string
	executionKind      string
	identityMethod     string
	implicitElevation  bool
	kubernetesUsername string
	mode               operationMode
	namespace          string
	namespaceAllowed   bool
	podSessionID       string
	principal          *authn.Principal
	pvcName            string
	route              string
	viewerSessionID    string
}

func WithDebugConfig(debug config.DebugConfig) HandlerOption {
	return func(h *Handler) {
		h.debug = debug
	}
}

func WithFeatureConfig(features config.FeatureConfig) HandlerOption {
	return func(h *Handler) {
		h.features = features
	}
}

func WithStorageQuotaService(storageQuota storageQuotaService) HandlerOption {
	return func(h *Handler) {
		h.storageQuota = storageQuota
	}
}

func WithStorageClassService(storageClasses storageClassService) HandlerOption {
	return func(h *Handler) {
		h.storageClasses = storageClasses
	}
}

func WithAdminAuthorizer(authz adminAuthorizer) HandlerOption {
	return func(h *Handler) {
		h.adminAuthz = authz
	}
}

func WithManagementRESTConfig(restConfig *rest.Config) HandlerOption {
	return func(h *Handler) {
		h.managementRESTConfig = restConfig
	}
}

func NewHandler(
	viewers viewerService,
	pods podService,
	auth authService,
	managementClient kubernetes.Interface,
	recorder *observability.Recorder,
	authz authorizer,
	options ...HandlerOption,
) *Handler {
	handler := &Handler{
		viewers:        viewers,
		storageClasses: unavailableStorageClassService{},
		pods:           pods,
		auth:           auth,
		storageQuota:   disabledStorageQuotaService{},
		recorder:       recorder,
		authz:          authz,
		adminAuthz:     denyAdminAuthorizer{},
		features:       config.Default().Features(),
	}
	for _, option := range options {
		option(handler)
	}
	if authz == nil {
		authz = newKubernetesAuthorizer(managementClient, recorder, handler.managementRESTConfig)
	}
	handler.authz = authz
	return handler
}

func clientConfigForPrincipal(canonical *rest.Config, principal *authn.Principal) *rest.Config {
	if principal == nil {
		return nil
	}
	if principal.ClientConfig == nil || canonical == nil {
		return principal.ClientConfig
	}

	user := principal.ClientConfig
	merged := rest.CopyConfig(canonical)
	merged.Username = user.Username
	merged.Password = user.Password
	merged.BearerToken = user.BearerToken
	merged.BearerTokenFile = user.BearerTokenFile
	merged.CertFile = user.CertFile
	merged.KeyFile = user.KeyFile
	merged.CertData = append([]byte(nil), user.CertData...)
	merged.KeyData = append([]byte(nil), user.KeyData...)
	merged.Impersonate = user.Impersonate
	merged.AuthProvider = user.AuthProvider
	merged.AuthConfigPersister = user.AuthConfigPersister
	merged.ExecProvider = user.ExecProvider
	return merged
}
