package viewer

import (
	"log/slog"
	"os"
	"sync"

	"github.com/nixieboluo/sealos-stroage-manager/internal/config"
	"github.com/nixieboluo/sealos-stroage-manager/internal/filebrowser"
	"github.com/nixieboluo/sealos-stroage-manager/internal/kube"
	"github.com/nixieboluo/sealos-stroage-manager/internal/observability"
	"github.com/nixieboluo/sealos-stroage-manager/internal/session"
	"github.com/nixieboluo/sealos-stroage-manager/internal/state"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var runtimeOnce sync.Once

func runtimeHandler() *Handler {
	runtimeOnce.Do(func() {
		handler, err := buildRuntimeHandler()
		if err != nil {
			slog.Error("viewer runtime unavailable", "error", err)
			return
		}
		defaultHandler = handler
	})
	if defaultHandler != nil {
		return defaultHandler
	}
	return NewHandler(unavailableViewerService{}, unavailablePodService{}, unavailableAuthService{}, nil)
}

func buildRuntimeHandler() (*Handler, error) {
	cfg, err := config.LoadFile(config.DefaultPath)
	if err != nil {
		return nil, err
	}
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	recorder := observability.New(cfg.Observability, os.Stdout)
	store := state.New(cfg.Cache)
	kubeClient := kube.New(clientset)
	pods := session.NewPodService(cfg, store, kubeClient, recorder)
	auth := session.NewAuthService(
		cfg,
		store,
		filebrowser.NewClient(cfg.Viewer.FileBrowser.LoginTimeout),
		recorder,
	)
	viewers := session.NewViewerService(cfg, store, kubeClient, pods, auth, recorder)
	return NewHandler(viewers, pods, auth, recorder), nil
}
