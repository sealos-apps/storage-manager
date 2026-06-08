package session

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	labelComponent      = "storage-management.sealos.io/component"
	labelPVCName        = "storage-management.sealos.io/pvc-name"
	labelPVCUID         = "storage-management.sealos.io/pvc-uid"
	labelPodSessionID   = "storage-management.sealos.io/pod-session-id"
	labelRuntimeVersion = "storage-management.sealos.io/runtime-version"
	componentViewer     = "viewer"

	annotationAccessMode     = "storage-management.sealos.io/access-mode"
	annotationCreatedAt      = "storage-management.sealos.io/created-at"
	annotationKeepaliveUntil = "storage-management.sealos.io/keepalive-until"
	annotationLastActiveAt   = "storage-management.sealos.io/last-active-at"
	annotationMode           = "storage-management.sealos.io/mode"
	annotationRuntimeVersion = "storage-management.sealos.io/runtime-version"
)

type PodService struct {
	cfg            config.Config
	store          *state.Store
	client         kube.Interface
	mounts         *kube.PVCMountDetector
	recorder       *observability.Recorder
	runtimeVersion string
	now            func() time.Time
}

type EnsurePodSessionInput struct {
	AdminContext bool
	Namespace    string
	PVCName      string
	PVCUID       string
	AccessMode   string
	Mode         string
	MountInfo    *domain.PVCMountInfo
}

func NewPodService(
	cfg config.Config,
	store *state.Store,
	client kube.Interface,
	recorder *observability.Recorder,
) *PodService {
	return &PodService{
		cfg:            cfg,
		store:          store,
		client:         client,
		mounts:         kube.NewPVCMountDetector(client),
		recorder:       recorder,
		runtimeVersion: runtimeVersion(cfg),
		now:            time.Now,
	}
}

func (s *PodService) MountDetector() *kube.PVCMountDetector {
	return s.mounts
}

func (s *PodService) EnsurePodSession(
	ctx context.Context,
	input EnsurePodSessionInput,
) (podSession *domain.PodSession, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pod.ensure_session",
		slog.String("namespace", input.Namespace),
		slog.String("pvc_name", input.PVCName),
		slog.String("access_mode", input.AccessMode),
		slog.String("mode", input.Mode),
	)
	defer func() {
		finish(err)
	}()

	now := s.now()
	if session, ok := s.store.FindPodSessionByPVC(input.Namespace, input.PVCUID, now); ok {
		if session.RuntimeVersion == s.runtimeVersion &&
			session.AdminContext == input.AdminContext &&
			session.Status != domain.PodStatusTerminated &&
			now.Before(session.ExpiresAt) {
			s.recorder.ObservePodSession("reused")
			s.recorder.Logger().LogAttrs(ctx, slog.LevelDebug, "pod.session_reused",
				slog.String("pod_session_id", session.ID),
				slog.String("namespace", session.Namespace),
				slog.String("pvc_name", session.PVCName),
				slog.String("runtime_version", session.RuntimeVersion),
				slog.String("status", session.Status),
				slog.String("source", "state"),
			)
			return session, nil
		}
	}

	existing, err := s.findExistingViewerPod(ctx, input.Namespace, input.PVCUID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if err := s.deletePodIfExists(ctx, existing.Namespace, existing.Name); err != nil {
			return nil, err
		}
		s.recorder.ObserveCleanupDeleted()
		s.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "pod.orphan_viewer_pod_deleted",
			slog.String("namespace", existing.Namespace),
			slog.String("pod_name", existing.Name),
			slog.String("pvc_uid", input.PVCUID),
		)
	}

	id, err := newID("ps")
	if err != nil {
		return nil, err
	}
	name := resourceName("viewer-" + id)
	viewerURL, err := s.viewerURL(id)
	if err != nil {
		return nil, err
	}
	podSession = &domain.PodSession{
		ID:                id,
		Namespace:         input.Namespace,
		PVCName:           input.PVCName,
		PVCUID:            input.PVCUID,
		AccessMode:        input.AccessMode,
		Mode:              input.Mode,
		PodName:           name,
		ServiceName:       name,
		ViewerURL:         viewerURL,
		InternalViewerURL: s.internalViewerURL(input.Namespace, name),
		RuntimeVersion:    s.runtimeVersion,
		Status:            domain.PodStatusCreating,
		CreatedAt:         now,
		UpdatedAt:         now,
		LastActiveAt:      now,
		ExpiresAt:         now.Add(s.cfg.Sessions.PodKeepaliveGrace),
		AdminContext:      input.AdminContext,
	}

	pod := s.buildPod(podSession, input.MountInfo)
	createdPod, err := s.client.CreatePod(ctx, pod)
	if err != nil {
		if apierrors.IsForbidden(err) {
			return nil, apienv.NewError(403, apienv.CodeViewerPodFailed, err.Error(), nil)
		}
		return nil, err
	}
	owner := podOwnerReference(createdPod)
	hookConfigMap := s.buildHookConfigMap(podSession, owner)
	if _, err := s.client.CreateConfigMap(ctx, hookConfigMap); err != nil {
		_ = s.client.DeletePod(ctx, createdPod.Namespace, createdPod.Name)
		return nil, err
	}
	service := s.buildService(podSession, owner)
	ingress, err := s.buildIngress(podSession, owner)
	if err != nil {
		_ = s.client.DeletePod(ctx, createdPod.Namespace, createdPod.Name)
		return nil, err
	}
	if _, err := s.client.CreateService(ctx, service); err != nil {
		_ = s.client.DeletePod(ctx, createdPod.Namespace, createdPod.Name)
		return nil, err
	}
	if _, err := s.client.CreateIngress(ctx, ingress); err != nil {
		_ = s.client.DeletePod(ctx, createdPod.Namespace, createdPod.Name)
		return nil, err
	}

	s.store.PutPodSession(podSession)
	s.recorder.ObservePodSession("created")
	s.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "pod.session_created",
		slog.String("pod_session_id", podSession.ID),
		slog.String("namespace", podSession.Namespace),
		slog.String("pod_name", podSession.PodName),
		slog.String("pvc_name", podSession.PVCName),
		slog.String("runtime_version", podSession.RuntimeVersion),
		slog.String("mode", podSession.Mode),
	)
	return podSession, nil
}

func (s *PodService) buildHookConfigMap(
	session *domain.PodSession,
	owner metav1.OwnerReference,
) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       session.Namespace,
			Name:            hookConfigMapName(session),
			Labels:          managedLabels(session),
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Data: map[string]string{
			"filebrowser-auth-hook.sh": s.cfg.Viewer.HookScript,
		},
	}
}

func (s *PodService) SyncPodStatus(
	ctx context.Context,
	podSession *domain.PodSession,
) (synced *domain.PodSession, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pod.sync_status",
		slog.String("pod_session_id", podSession.ID),
		slog.String("namespace", podSession.Namespace),
		slog.String("pod_name", podSession.PodName),
	)
	defer func() {
		finish(err)
	}()

	pod, err := s.client.GetPod(ctx, podSession.Namespace, podSession.PodName)
	if err != nil {
		return nil, err
	}
	updated := *podSession
	updated.UpdatedAt = s.now()
	updated.NodeName = pod.Spec.NodeName
	switch pod.Status.Phase {
	case corev1.PodRunning:
		if podReady(pod) {
			updated.Status = domain.PodStatusReady
			updated.Reason = ""
			break
		}
		if reason := containerFailureReason(pod); reason != "" {
			updated.Status = domain.PodStatusFailed
			updated.Reason = reason
		}
	case corev1.PodFailed:
		updated.Status = domain.PodStatusFailed
		updated.Reason = pod.Status.Reason
	case corev1.PodSucceeded:
		updated.Status = domain.PodStatusTerminated
	default:
		updated.Status = domain.PodStatusCreating
	}
	s.recorder.Logger().LogAttrs(ctx, slog.LevelDebug, "pod.status_synced",
		slog.String("pod_session_id", updated.ID),
		slog.String("namespace", updated.Namespace),
		slog.String("pod_name", updated.PodName),
		slog.String("status", updated.Status),
		slog.String("node_name", updated.NodeName),
		slog.String("reason", updated.Reason),
	)
	return &updated, nil
}

func containerFailureReason(pod *corev1.Pod) string {
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
			switch status.State.Waiting.Reason {
			case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError":
				return status.State.Waiting.Reason
			}
		}
		if status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode != 0 {
			reason := status.LastTerminationState.Terminated.Reason
			if reason == "" {
				reason = fmt.Sprintf("ContainerExited%d", status.LastTerminationState.Terminated.ExitCode)
			}
			return reason
		}
	}
	return ""
}

func (s *PodService) ClosePodSession(
	ctx context.Context,
	podSessionID string,
) (closed *domain.PodSession, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pod.close_session",
		slog.String("pod_session_id", podSessionID),
	)
	defer func() {
		finish(err)
	}()

	now := s.now()
	podSession, ok := s.store.GetPodSessionIncludingExpired(podSessionID)
	if !ok {
		return nil, fmt.Errorf("pod session %q not found", podSessionID)
	}
	podSession.Status = domain.PodStatusTerminating
	podSession.UpdatedAt = now
	s.store.PutPodSession(podSession)

	if err := s.deletePodIfExists(ctx, podSession.Namespace, podSession.PodName); err != nil {
		return nil, fmt.Errorf("closing pod session %q: %w", podSessionID, err)
	}

	podSession.Status = domain.PodStatusTerminated
	podSession.UpdatedAt = now
	s.store.DeletePodSession(podSessionID)
	s.recorder.ObservePodSession("deleted")
	s.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "pod.session_closed",
		slog.String("pod_session_id", podSession.ID),
		slog.String("namespace", podSession.Namespace),
		slog.String("pod_name", podSession.PodName),
	)
	return podSession, nil
}

func (s *PodService) RefreshPodSessionKeepalive(
	ctx context.Context,
	podSession *domain.PodSession,
) (refreshed *domain.PodSession, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pod.refresh_keepalive",
		slog.String("pod_session_id", podSession.ID),
		slog.String("namespace", podSession.Namespace),
		slog.String("pod_name", podSession.PodName),
	)
	defer func() {
		finish(err)
	}()

	now := s.now()
	updated := *podSession
	updated.LastActiveAt = now
	updated.ExpiresAt = now.Add(s.cfg.Sessions.PodKeepaliveGrace)
	if _, err := s.client.PatchPodAnnotations(
		ctx,
		updated.Namespace,
		updated.PodName,
		keepaliveAnnotations(&updated),
	); err != nil {
		return nil, err
	}
	s.store.PutPodSession(&updated)
	return &updated, nil
}

func (s *PodService) deletePodIfExists(ctx context.Context, namespace string, name string) error {
	if err := s.client.DeletePod(ctx, namespace, name); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
