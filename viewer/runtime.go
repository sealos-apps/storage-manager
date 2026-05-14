package viewer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"encore.dev/cron"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/filebrowser"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var runtimeOnce sync.Once
var runtimeCleanup *session.CleanupService
var _ = cron.NewJob("viewer-cleanup", cron.JobConfig{
	Title:    "Clean up idle File Browser viewer sessions",
	Every:    1 * cron.Minute,
	Endpoint: CleanupViewerState,
})

type Runtime struct {
	Handler *Handler
	cleanup *session.CleanupService
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

func runtimeHandler() *Handler {
	runtimeOnce.Do(func() {
		runtime, err := NewRuntime(config.DefaultPath)
		if err != nil {
			slog.Error("viewer runtime unavailable", "error", err)
			return
		}
		defaultHandler = runtime.Handler
		runtimeCleanup = runtime.cleanup
	})
	if defaultHandler != nil {
		return defaultHandler
	}
	return NewHandler(
		unavailableViewerService{},
		unavailablePodService{},
		unavailableAuthService{},
		nil,
		observability.New(config.Default().Observability, nil),
		denyAuthorizer{},
	)
}

func newRuntimeFromConfig(cfg config.Config) (*Runtime, error) {
	restConfig, err := managementRESTConfig(cfg)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("building management kubernetes client: %w", err)
	}
	recorder := observability.New(cfg.Observability, os.Stdout)
	store := state.New(cfg.Cache)
	kubeClient := kube.WithObservability(kube.New(clientset), recorder)
	pods := session.NewPodService(cfg, store, kubeClient, recorder)
	auth := session.NewAuthService(
		cfg,
		store,
		filebrowser.NewClient(cfg.Viewer.FileBrowser.LoginTimeout),
		recorder,
	)
	viewers := session.NewViewerService(cfg, store, kubeClient, pods, auth, recorder)
	cleanup := session.NewCleanupService(cfg, store, pods, recorder)
	handler := NewHandler(
		viewers,
		pods,
		auth,
		clientset,
		recorder,
		nil,
	)
	return &Runtime{
		Handler: handler,
		cleanup: cleanup,
	}, nil
}

func managementRESTConfig(cfg config.Config) (*rest.Config, error) {
	path := cfg.Kubernetes.ManagementKubeconfigPath
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

//encore:api private
func CleanupViewerState(ctx context.Context) error {
	runtimeOnce.Do(func() {
		runtime, err := NewRuntime(config.DefaultPath)
		if err != nil {
			slog.Error("viewer runtime unavailable", "error", err)
			return
		}
		defaultHandler = runtime.Handler
		runtimeCleanup = runtime.cleanup
	})
	if runtimeCleanup == nil {
		return nil
	}
	return runtimeCleanup.RunOnce(ctx)
}
