package session

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type ViewerService struct {
	cfg        config.Config
	store      *state.Store
	kube       kube.Interface
	pods       *PodService
	auth       *AuthService
	recorder   *observability.Recorder
	pvcMetrics pvcMetricsReader
	now        func() time.Time
}

type ViewerServiceOption func(*ViewerService)

type pvcMetricsReader interface {
	ListPVCVolumeStats(ctx context.Context, namespace string) (map[string]domain.PVCVolumeStats, error)
}

func WithPVCMetrics(metrics pvcMetricsReader) ViewerServiceOption {
	return func(s *ViewerService) {
		s.pvcMetrics = metrics
	}
}

type CreateViewerSessionInput struct {
	AdminContext bool
	Namespace    string
	PVCName      string
	UserID       string
}

func NewViewerService(
	cfg config.Config,
	store *state.Store,
	kubeClient kube.Interface,
	podService *PodService,
	authService *AuthService,
	recorder *observability.Recorder,
	options ...ViewerServiceOption,
) *ViewerService {
	service := &ViewerService{
		cfg:      cfg,
		store:    store,
		kube:     kubeClient,
		pods:     podService,
		auth:     authService,
		recorder: recorder,
		now:      time.Now,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *ViewerService) CreateViewerSession(
	ctx context.Context,
	input CreateViewerSessionInput,
) (viewer *domain.ViewerSession, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"viewer.create_session",
		slog.String("namespace", input.Namespace),
		slog.String("pvc_name", input.PVCName),
	)
	defer func() {
		finish(err)
	}()

	pvc, err := s.kube.GetPVC(ctx, input.Namespace, input.PVCName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodePVCNotFound, "PVC not found", nil)
		}
		return nil, err
	}
	accessModes := make([]string, 0, len(pvc.Spec.AccessModes))
	for _, mode := range pvc.Spec.AccessModes {
		accessModes = append(accessModes, string(mode))
	}
	supported, viewerMode, reason := kube.ViewerSupportForAccessModes(accessModes)
	if !supported {
		return nil, apienv.NewError(400, apienv.CodeUnsupportedAccessMode, reason, nil)
	}
	mountInfo, err := s.detectPVCMounts(ctx, input.Namespace, input.PVCName)
	if err != nil {
		return nil, err
	}
	if slices.Contains(accessModes, domain.AccessModeReadWriteOnce) {
		if mountInfo.Conflict {
			return nil, apienv.NewError(409, apienv.CodePVCMountConflict, "PVC is mounted on multiple nodes", nil)
		}
		if mountInfo.Reason == "PVC_MOUNT_PENDING" {
			return nil, apienv.NewError(409, apienv.CodePVCMountPending, "PVC is referenced by pending pods", nil)
		}
	}
	podSession, err := s.pods.EnsurePodSession(ctx, EnsurePodSessionInput{
		AdminContext: input.AdminContext,
		Namespace:    input.Namespace,
		PVCName:      pvc.Name,
		PVCUID:       string(pvc.UID),
		AccessMode:   primaryAccessMode(accessModes),
		Mode:         viewerMode,
		MountInfo:    mountInfo,
	})
	if err != nil {
		return nil, err
	}
	now := s.now()
	id, err := newID("vs")
	if err != nil {
		return nil, err
	}
	viewer = &domain.ViewerSession{
		ID:              id,
		PodSessionID:    podSession.ID,
		Namespace:       input.Namespace,
		PVCName:         pvc.Name,
		UserID:          input.UserID,
		Username:        id,
		Permission:      viewerMode,
		Status:          statusFromPod(podSession.Status),
		PodStatus:       podSession.Status,
		ViewerURL:       podSession.ViewerURL,
		Mode:            podSession.Mode,
		Reason:          podSession.Reason,
		TokenReady:      podSession.Status == domain.PodStatusReady,
		CreatedAt:       now,
		LastHeartbeatAt: now,
		ExpiresAt:       now.Add(s.cfg.Sessions.ViewerSessionTimout),
		AdminContext:    input.AdminContext,
	}
	s.store.PutViewerSession(viewer)
	s.recorder.ObserveViewerSession("created")
	s.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "viewer.session_created",
		slog.String("viewer_session_id", viewer.ID),
		slog.String("pod_session_id", podSession.ID),
		slog.String("namespace", input.Namespace),
		slog.String("pvc_name", pvc.Name),
		slog.String("mode", viewerMode),
		slog.String("status", viewer.Status),
	)
	return viewer, nil
}

func (s *ViewerService) GetViewerSession(
	ctx context.Context,
	id string,
	userID string,
) (viewer *domain.ViewerSession, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"viewer.get_session",
		slog.String("viewer_session_id", id),
	)
	defer func() {
		finish(err)
	}()

	now := s.now()
	viewer, ok := s.store.GetViewerSession(id, now)
	if !ok {
		return nil, apienv.NewError(404, apienv.CodeViewerSessionNotFound, "Viewer session no longer exists", nil)
	}
	if viewer.UserID != userID {
		return nil, apienv.NewError(403, apienv.CodePVCAccessDenied, "Viewer session belongs to another user", nil)
	}
	pod, ok := s.store.GetPodSession(viewer.PodSessionID, now)
	if ok {
		if s.pods != nil {
			synced, err := s.pods.SyncPodStatus(ctx, pod)
			if err == nil {
				pod = synced
				s.store.PutPodSession(pod)
			}
		}
		viewer.PodStatus = pod.Status
		viewer.Status = statusFromPod(pod.Status)
		viewer.ViewerURL = pod.ViewerURL
		viewer.Mode = pod.Mode
		viewer.Reason = pod.Reason
		viewer.TokenReady = pod.Status == domain.PodStatusReady
		s.store.PutViewerSession(viewer)
	}
	return viewer, nil
}

func (s *ViewerService) IssueToken(
	ctx context.Context,
	id string,
	userID string,
) (token *domain.ViewerToken, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"viewer.issue_token",
		slog.String("viewer_session_id", id),
	)
	defer func() {
		finish(err)
	}()

	now := s.now()
	viewer, ok := s.store.GetViewerSession(id, now)
	if !ok {
		return nil, apienv.NewError(404, apienv.CodeViewerSessionNotFound, "Viewer session no longer exists", nil)
	}
	if viewer.UserID != userID {
		return nil, apienv.NewError(403, apienv.CodePVCAccessDenied, "Viewer session belongs to another user", nil)
	}
	pod, ok := s.store.GetPodSession(viewer.PodSessionID, now)
	if !ok && s.pods != nil {
		if replacement, err := s.replaceMissingPodSessionForViewer(ctx, viewer); err == nil {
			pod = replacement
			ok = true
			viewer.PodSessionID = replacement.ID
			viewer.PodStatus = replacement.Status
			viewer.Status = statusFromPod(replacement.Status)
			viewer.ViewerURL = replacement.ViewerURL
			viewer.Mode = replacement.Mode
			viewer.Reason = replacement.Reason
			viewer.TokenReady = replacement.Status == domain.PodStatusReady
			s.store.PutViewerSession(viewer)
		} else {
			return nil, err
		}
	}
	if ok && s.pods != nil {
		synced, err := s.pods.SyncPodStatus(ctx, pod)
		if err == nil {
			pod = synced
			s.store.PutPodSession(pod)
		}
	}
	if !ok || pod.Status != domain.PodStatusReady {
		return nil, apienv.NewError(409, apienv.CodeViewerPodCreating, "Viewer pod is not ready", nil)
	}
	return s.auth.IssueToken(ctx, viewer, pod)
}

func (s *ViewerService) replaceMissingPodSessionForViewer(
	ctx context.Context,
	viewer *domain.ViewerSession,
) (*domain.PodSession, error) {
	if viewer.Namespace == "" || viewer.PVCName == "" {
		return nil, apienv.NewError(404, apienv.CodePodSessionNotFound, "Pod session no longer exists", nil)
	}
	pvc, err := s.kube.GetPVC(ctx, viewer.Namespace, viewer.PVCName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodePVCNotFound, "PVC not found", nil)
		}
		return nil, err
	}
	accessModes := make([]string, 0, len(pvc.Spec.AccessModes))
	for _, mode := range pvc.Spec.AccessModes {
		accessModes = append(accessModes, string(mode))
	}
	supported, viewerMode, reason := kube.ViewerSupportForAccessModes(accessModes)
	if !supported {
		return nil, apienv.NewError(400, apienv.CodeUnsupportedAccessMode, reason, nil)
	}
	mountInfo, err := s.detectPVCMounts(ctx, viewer.Namespace, viewer.PVCName)
	if err != nil {
		return nil, err
	}
	if err := s.pods.DeleteViewerPodsBySessionID(ctx, viewer.Namespace, viewer.PodSessionID); err != nil {
		return nil, err
	}
	return s.pods.EnsurePodSession(ctx, EnsurePodSessionInput{
		AdminContext: viewer.AdminContext,
		Namespace:    viewer.Namespace,
		PVCName:      pvc.Name,
		PVCUID:       string(pvc.UID),
		AccessMode:   primaryAccessMode(accessModes),
		Mode:         viewerMode,
		MountInfo:    mountInfo,
	})
}

func (s *ViewerService) Heartbeat(ctx context.Context, id string) (*domain.Heartbeat, error) {
	return s.HeartbeatForUser(ctx, id, "")
}

func (s *ViewerService) HeartbeatForUser(ctx context.Context, id string, userID string) (*domain.Heartbeat, error) {
	now := s.now()
	viewer, ok := s.store.GetViewerSession(id, now)
	if !ok {
		return nil, apienv.NewError(404, apienv.CodeViewerSessionNotFound, "Viewer session no longer exists", nil)
	}
	if userID != "" && viewer.UserID != userID {
		return nil, apienv.NewError(403, apienv.CodePVCAccessDenied, "Viewer session belongs to another user", nil)
	}
	viewer.LastHeartbeatAt = now
	viewer.ExpiresAt = now.Add(s.cfg.Sessions.ViewerSessionTimout)
	s.store.PutViewerSession(viewer)
	if pod, ok := s.store.GetPodSession(viewer.PodSessionID, now); ok && s.pods != nil {
		if _, err := s.pods.RefreshPodSessionKeepalive(ctx, pod); err != nil {
			return nil, err
		}
	}
	return &domain.Heartbeat{
		ViewerSessionID: viewer.ID,
		Status:          domain.ViewerStatusActive,
		ServerTime:      now,
		ExpiresAt:       viewer.ExpiresAt,
	}, nil
}

func (s *ViewerService) CloseViewerSession(id string) (*domain.ViewerSession, error) {
	return s.CloseViewerSessionForUser(id, "")
}

func (s *ViewerService) CloseViewerSessionForUser(id string, userID string) (*domain.ViewerSession, error) {
	now := s.now()
	viewer, ok := s.store.GetViewerSession(id, now)
	if !ok {
		return nil, apienv.NewError(404, apienv.CodeViewerSessionNotFound, "Viewer session no longer exists", nil)
	}
	if userID != "" && viewer.UserID != userID {
		return nil, apienv.NewError(403, apienv.CodePVCAccessDenied, "Viewer session belongs to another user", nil)
	}
	viewer.Status = domain.ViewerStatusClosed
	viewer.ExpiresAt = now
	s.store.DeleteViewerSession(id)
	s.recorder.ObserveViewerSession("closed")
	return viewer, nil
}

func (s *ViewerService) GetPodSession(id string) (*domain.PodSession, error) {
	pod, ok := s.store.GetPodSession(id, s.now())
	if !ok {
		return nil, apienv.NewError(404, apienv.CodePodSessionNotFound, "Pod session no longer exists", nil)
	}
	return pod, nil
}

func statusFromPod(podStatus string) string {
	switch podStatus {
	case domain.PodStatusReady:
		return domain.ViewerStatusReady
	case domain.PodStatusFailed:
		return domain.ViewerStatusFailed
	case domain.PodStatusTerminated:
		return domain.ViewerStatusClosed
	default:
		return domain.PodStatusCreating
	}
}
