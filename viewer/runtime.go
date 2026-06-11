package viewer

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/filebrowser"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/pvcmetrics"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var runtimeOnce sync.Once
var startRuntimeCleanupLoop = startCleanupLoop

type Runtime struct {
	Handler  *Handler
	cleanup  *session.CleanupService
	recorder *observability.Recorder
	cancel   context.CancelFunc
}

func NewRuntime(configPath string) (*Runtime, error) {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return nil, err
	}
	return newRuntimeFromConfig(cfg)
}

func (r *Runtime) Cleanup(ctx context.Context) error {
	if r == nil || r.cleanup == nil {
		return nil
	}
	return r.cleanup.RunOnce(ctx)
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r != nil && r.cancel != nil {
		r.cancel()
	}
	if r == nil || r.recorder == nil {
		return nil
	}
	return r.recorder.Shutdown(ctx)
}

func runtimeHandler() *Handler {
	runtimeOnce.Do(func() {
		runtime, err := NewRuntime("")
		if err != nil {
			slog.Error("viewer runtime unavailable", "error", err)
			return
		}
		defaultHandler = runtime.Handler
	})
	if defaultHandler != nil {
		return defaultHandler
	}
	return NewHandler(
		unavailableViewerService{},
		unavailablePodService{},
		unavailableAuthService{},
		nil,
		observability.MustNew(config.Default().Observability, nil),
		denyAuthorizer{},
		WithStorageClassService(unavailableStorageClassService{}),
	)
}

func newRuntimeFromConfig(cfg config.Config) (*Runtime, error) {
	recorder, err := observability.New(
		context.Background(),
		cfg.Observability,
		os.Stdout,
		observability.WithMetrics(encoreMetricSources()),
	)
	if err != nil {
		return nil, fmt.Errorf("configuring observability: %w", err)
	}
	restConfig, err := managementRESTConfig(cfg)
	if err != nil {
		_ = recorder.Shutdown(context.Background())
		return nil, err
	}
	wrapKubernetesTransport(restConfig, recorder.OTelTracerProvider())
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		_ = recorder.Shutdown(context.Background())
		return nil, fmt.Errorf("building management kubernetes client: %w", err)
	}
	tracerProvider := recorder.OTelTracerProvider()
	store := state.New(cfg.Cache)
	kubeClient := kube.WithObservability(kube.New(clientset), recorder)
	adminKubeClient, err := storageClassAdminKubeClient(cfg, restConfig, recorder)
	if err != nil {
		_ = recorder.Shutdown(context.Background())
		return nil, err
	}
	pods := session.NewPodService(cfg, store, kubeClient, recorder)
	auth := session.NewAuthService(
		cfg,
		store,
		filebrowser.NewObservedClient(cfg.Viewer.FileBrowser.LoginTimeout, tracerProvider),
		recorder,
	)
	viewers := session.NewViewerService(
		cfg,
		store,
		kubeClient,
		pods,
		auth,
		recorder,
		session.WithPVCMetrics(newPVCMetricsReader(cfg.Viewer.PVCMetrics, recorder)),
	)
	storageClasses := session.NewStorageClassService(adminKubeClient, recorder)
	cleanup := session.NewCleanupService(cfg, store, pods, recorder)
	// This in-process scheduler assumes a single backend replica. If the backend
	// becomes horizontally scaled, replace it with leader election or a Kubernetes
	// CronJob so multiple replicas do not run cleanup concurrently.
	cancelCleanup := startRuntimeCleanupLoop(context.Background(), cfg.Cache.PurgeInterval, cleanup.RunOnce, logCleanupError)
	handler := NewHandler(
		viewers,
		pods,
		auth,
		clientset,
		recorder,
		nil,
		WithDebugConfig(cfg.Debug),
		WithFeatureConfig(cfg.Features()),
		WithStorageClassService(storageClasses),
		WithAdminAuthorizer(newKubernetesAdminAuthorizer(cfg.Admin, recorder)),
	)
	return &Runtime{
		Handler:  handler,
		cleanup:  cleanup,
		recorder: recorder,
		cancel:   cancelCleanup,
	}, nil
}

func newPVCMetricsReader(
	cfg config.PVCMetricsConfig,
	recorder *observability.Recorder,
) pvcmetrics.Reader {
	if !cfg.Enabled {
		return nil
	}
	return pvcmetrics.NewClient(cfg, &http.Client{
		Timeout:   cfg.QueryTimeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}, recorder)
}

func storageClassAdminKubeClient(
	cfg config.Config,
	base *rest.Config,
	recorder *observability.Recorder,
) (kube.Interface, error) {
	if cfg.Debug.Enabled && strings.TrimSpace(cfg.Debug.ManagementKubeconfigPath) != "" {
		adminConfig := rest.CopyConfig(base)
		wrapKubernetesTransport(adminConfig, recorder.OTelTracerProvider())
		clientset, err := kubernetes.NewForConfig(adminConfig)
		if err != nil {
			return nil, fmt.Errorf("building debug storageclass admin kubernetes client: %w", err)
		}
		return kube.WithObservability(kube.New(clientset), recorder), nil
	}
	namespace, err := backendNamespace()
	if err != nil {
		if cfg.Debug.Enabled {
			namespace = "sealos-storage-manager"
		} else {
			return nil, err
		}
	}
	adminConfig := rest.CopyConfig(base)
	adminConfig.Impersonate.UserName = storageClassAdminUsername(namespace, cfg.Admin.StorageClassServiceAccountName)
	wrapKubernetesTransport(adminConfig, recorder.OTelTracerProvider())
	clientset, err := kubernetes.NewForConfig(adminConfig)
	if err != nil {
		return nil, fmt.Errorf("building storageclass admin kubernetes client: %w", err)
	}
	return kube.WithObservability(kube.New(clientset), recorder), nil
}

func storageClassAdminUsername(namespace string, serviceAccountName string) string {
	return "system:serviceaccount:" + namespace + ":" + serviceAccountName
}

var serviceAccountNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

func backendNamespace() (string, error) {
	body, err := os.ReadFile(serviceAccountNamespacePath)
	if err != nil {
		return "", fmt.Errorf("reading backend service account namespace: %w", err)
	}
	namespace := strings.TrimSpace(string(body))
	if namespace == "" {
		return "", fmt.Errorf("backend service account namespace is empty")
	}
	return namespace, nil
}

func wrapKubernetesTransport(restConfig *rest.Config, provider trace.TracerProvider) {
	if restConfig == nil || provider == nil {
		return
	}
	restConfig.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(
			rt,
			otelhttp.WithTracerProvider(provider),
			otelhttp.WithMeterProvider(noop.NewMeterProvider()),
			otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			)),
			otelhttp.WithSpanNameFormatter(func(_ string, req *http.Request) string {
				return "kubernetes.http." + req.Method
			}),
		)
	})
}

func managementRESTConfig(cfg config.Config) (*rest.Config, error) {
	path := ""
	if cfg.Debug.Enabled {
		path = cfg.Debug.ManagementKubeconfigPath
	}
	if path == "" {
		restConfig, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("loading in-cluster management config: %w", err)
		}
		return restConfig, nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Clean(path)
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("loading management kubeconfig: %w", err)
	}
	return restConfig, nil
}
