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
)

type ViewerService struct {
	cfg      config.Config
	store    *state.Store
	kube     kube.Interface
	pods     *PodService
	auth     *AuthService
	recorder *observability.Recorder
	now      func() time.Time
}

type CreateViewerSessionInput struct {
	Namespace string
	PVCName   string
	UserID    string
}

func NewViewerService(
	cfg config.Config,
	store *state.Store,
	kubeClient kube.Interface,
	podService *PodService,
	authService *AuthService,
	recorder *observability.Recorder,
) *ViewerService {
	return &ViewerService{
		cfg:      cfg,
		store:    store,
		kube:     kubeClient,
		pods:     podService,
		auth:     authService,
		recorder: recorder,
		now:      time.Now,
	}
}

func (s *ViewerService) ListPVCs(ctx context.Context, namespace string) (items []domain.PVC, err error) {
	ctx, finish := s.recorder.StartSpan(ctx,
		"viewer.list_pvcs",
		slog.String("namespace", namespace),
	)
	defer func() {
		finish(err)
	}()

	pvcs, err := s.kube.ListPVCs(ctx, namespace)
	if err != nil {
		return nil, err
	}
	items = make([]domain.PVC, 0, len(pvcs))
	for _, pvc := range pvcs {
		accessModes := make([]string, 0, len(pvc.Spec.AccessModes))
		for _, mode := range pvc.Spec.AccessModes {
			accessModes = append(accessModes, string(mode))
		}
		mountInfo, err := s.detectPVCMounts(ctx, pvc.Namespace, pvc.Name)
		if err != nil {
			return nil, err
		}
		supported, viewerMode, reason := kube.ViewerSupportForAccessModes(accessModes)
		items = append(items, domain.PVC{
			Namespace:        pvc.Namespace,
			Name:             pvc.Name,
			UID:              string(pvc.UID),
			CapacityBytes:    pvc.Spec.Resources.Requests.Storage().Value(),
			Capacity:         pvc.Spec.Resources.Requests.Storage().String(),
			AccessModes:      accessModes,
			Mounted:          mountInfo.Mounted,
			MountedPods:      mountInfo.MountedPods,
			ViewerSupported:  supported,
			ViewerMode:       viewerMode,
			ViewerScheduling: kube.SchedulingForPVC(accessModes, mountInfo),
			Reason:           reason,
		})
	}
	s.recorder.Logger().LogAttrs(ctx, slog.LevelDebug, "viewer.list_pvcs.result",
		slog.String("namespace", namespace),
		slog.Int("pvc_count", len(items)),
	)
	return items, nil
}

func (s *ViewerService) CreateViewerSession(
	ctx context.Context,
	input CreateViewerSessionInput,
) (viewer *domain.ViewerSession, err error) {
	ctx, finish := s.recorder.StartSpan(ctx,
		"viewer.create_session",
		slog.String("namespace", input.Namespace),
		slog.String("pvc_name", input.PVCName),
	)
	defer func() {
		finish(err)
	}()

	if !s.namespaceAllowed(input.Namespace) {
		return nil, apienv.NewError(403, apienv.CodePVCAccessDenied, "Namespace is not allowed", nil)
	}
	pvc, err := s.kube.GetPVC(ctx, input.Namespace, input.PVCName)
	if err != nil {
		return nil, apienv.NewError(404, apienv.CodePVCNotFound, "PVC not found", nil)
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
		Namespace:  input.Namespace,
		PVCName:    pvc.Name,
		PVCUID:     string(pvc.UID),
		AccessMode: primaryAccessMode(accessModes),
		Mode:       viewerMode,
		MountInfo:  mountInfo,
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
	}
	s.store.PutViewerSession(viewer)
	s.recorder.Metrics().ViewerCreated.Add(1)
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
	ctx, finish := s.recorder.StartSpan(ctx,
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
	ctx, finish := s.recorder.StartSpan(ctx,
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
	if ok && s.pods != nil {
		synced, err := s.pods.SyncPodStatus(ctx, pod)
		if err == nil {
			pod = synced
		}
	}
	if !ok || pod.Status != domain.PodStatusReady {
		return nil, apienv.NewError(409, apienv.CodeViewerPodCreating, "Viewer pod is not ready", nil)
	}
	return s.auth.IssueToken(ctx, viewer, pod)
}

func (s *ViewerService) Heartbeat(id string) (*domain.Heartbeat, error) {
	return s.HeartbeatForUser(id, "")
}

func (s *ViewerService) HeartbeatForUser(id string, userID string) (*domain.Heartbeat, error) {
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
	if pod, ok := s.store.GetPodSession(viewer.PodSessionID, now); ok {
		pod.LastActiveAt = now
		pod.ExpiresAt = now.Add(s.cfg.Sessions.PodKeepaliveGrace)
		s.store.PutPodSession(pod)
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
	s.recorder.Metrics().ViewerClosed.Add(1)
	return viewer, nil
}

func (s *ViewerService) GetPodSession(id string) (*domain.PodSession, error) {
	pod, ok := s.store.GetPodSession(id, s.now())
	if !ok {
		return nil, apienv.NewError(404, apienv.CodeViewerSessionNotFound, "Pod session no longer exists", nil)
	}
	return pod, nil
}

func (s *ViewerService) detectPVCMounts(
	ctx context.Context,
	namespace string,
	pvcName string,
) (mountInfo *domain.PVCMountInfo, err error) {
	ctx, finish := s.recorder.StartSpan(ctx,
		"pvc.detect_mounts",
		slog.String("namespace", namespace),
		slog.String("pvc_name", pvcName),
	)
	defer func() {
		finish(err)
	}()

	mountInfo, err = s.pods.MountDetector().DetectPVCMounts(ctx, namespace, pvcName)
	if err != nil {
		return nil, err
	}
	s.recorder.Logger().LogAttrs(ctx, slog.LevelDebug, "pvc.mounts_detected",
		slog.String("namespace", namespace),
		slog.String("pvc_name", pvcName),
		slog.Bool("mounted", mountInfo.Mounted),
		slog.Bool("conflict", mountInfo.Conflict),
		slog.Int("mounted_pod_count", len(mountInfo.MountedPods)),
		slog.Int("node_count", len(mountInfo.Nodes)),
		slog.String("reason", mountInfo.Reason),
	)
	return mountInfo, nil
}

func (s *ViewerService) namespaceAllowed(namespace string) bool {
	if len(s.cfg.Viewer.NamespaceAllowlist) == 0 {
		return true
	}
	return slices.Contains(s.cfg.Viewer.NamespaceAllowlist, namespace)
}

func primaryAccessMode(accessModes []string) string {
	for _, mode := range []string{
		domain.AccessModeReadWriteMany,
		domain.AccessModeReadOnlyMany,
		domain.AccessModeReadWriteOnce,
		domain.AccessModeReadWriteOncePod,
	} {
		if slices.Contains(accessModes, mode) {
			return mode
		}
	}
	return ""
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
