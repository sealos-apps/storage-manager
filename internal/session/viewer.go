package session

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
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

type CreatePVCInput struct {
	Namespace        string
	Name             string
	Capacity         string
	CapacityBytes    int64
	AccessModes      []string
	StorageClassName string
}

type DeletePVCInput struct {
	Namespace string
	Name      string
}

type ExpandPVCInput struct {
	Namespace     string
	Name          string
	Capacity      string
	CapacityBytes int64
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
	ctx, finish := s.recorder.TraceOperation(ctx,
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

func (s *ViewerService) CreatePVC(ctx context.Context, input CreatePVCInput) (pvc *domain.PVC, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pvc.create",
		slog.String("namespace", input.Namespace),
		slog.String("pvc_name", input.Name),
	)
	defer func() {
		finish(err)
	}()

	storage, err := capacityQuantity(input.Capacity, input.CapacityBytes)
	if err != nil {
		return nil, apienv.NewError(400, apienv.CodeValidationError, err.Error(), nil)
	}
	accessModes, err := persistentVolumeAccessModes(input.AccessModes)
	if err != nil {
		return nil, apienv.NewError(400, apienv.CodeUnsupportedAccessMode, err.Error(), nil)
	}
	if errs := validation.IsDNS1123Label(input.Name); len(errs) > 0 {
		return nil, apienv.NewError(400, apienv.CodeValidationError, "PVC name must be a DNS-1123 label", map[string]any{
			"violations": errs,
		})
	}
	pvcSpec := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: input.Namespace,
			Name:      input.Name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "sealos-storage-manager",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: storage},
			},
		},
	}
	storageClassName := strings.TrimSpace(input.StorageClassName)
	if storageClassName == "" {
		return nil, apienv.NewError(400, apienv.CodeValidationError, "storage_class_name is required", nil)
	}
	storageClass, err := s.kube.GetStorageClass(ctx, storageClassName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodeStorageClassNotFound, "StorageClass not found", nil)
		}
		return nil, err
	}
	allowedModes, annotationStatus := StorageClassAccessPolicy(storageClass.Annotations)
	if annotationStatus != storageClassAnnotationReady {
		return nil, apienv.NewError(403, apienv.CodeStorageClassNotVisible, "StorageClass is not available for PVC creation", map[string]any{
			"annotation_status": annotationStatus,
		})
	}
	for _, mode := range input.AccessModes {
		if !slices.Contains(allowedModes, strings.TrimSpace(mode)) {
			return nil, apienv.NewError(400, apienv.CodeUnsupportedAccessMode, "Access mode is not allowed by the selected StorageClass", map[string]any{
				"allowed_access_modes": allowedModes,
			})
		}
	}
	pvcSpec.Spec.StorageClassName = &storageClassName
	created, err := s.kube.CreatePVC(ctx, pvcSpec)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, apienv.NewError(409, apienv.CodePVCAlreadyExists, "PVC already exists", nil)
		}
		return nil, err
	}
	return s.pvcToDomain(ctx, created)
}

func (s *ViewerService) DeletePVC(ctx context.Context, input DeletePVCInput) (pvc *domain.PVC, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pvc.delete",
		slog.String("namespace", input.Namespace),
		slog.String("pvc_name", input.Name),
	)
	defer func() {
		finish(err)
	}()

	current, err := s.kube.GetPVC(ctx, input.Namespace, input.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodePVCNotFound, "PVC not found", nil)
		}
		return nil, err
	}
	mountInfo, err := s.detectPVCMounts(ctx, input.Namespace, input.Name)
	if err != nil {
		return nil, err
	}
	if mountInfo.Mounted {
		return nil, apienv.NewError(409, apienv.CodePVCInUse, "PVC is still mounted", map[string]any{
			"mounted_pods": mountInfo.MountedPods,
		})
	}
	deleted, err := s.pvcToDomain(ctx, current)
	if err != nil {
		return nil, err
	}
	if err := s.kube.DeletePVC(ctx, input.Namespace, input.Name); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodePVCNotFound, "PVC not found", nil)
		}
		return nil, err
	}
	return deleted, nil
}

func (s *ViewerService) ExpandPVC(ctx context.Context, input ExpandPVCInput) (pvc *domain.PVC, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pvc.expand",
		slog.String("namespace", input.Namespace),
		slog.String("pvc_name", input.Name),
	)
	defer func() {
		finish(err)
	}()

	target, err := capacityQuantity(input.Capacity, input.CapacityBytes)
	if err != nil {
		return nil, apienv.NewError(400, apienv.CodeValidationError, err.Error(), nil)
	}
	current, err := s.kube.GetPVC(ctx, input.Namespace, input.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodePVCNotFound, "PVC not found", nil)
		}
		return nil, err
	}
	currentStorage := current.Spec.Resources.Requests.Storage()
	if currentStorage == nil || currentStorage.IsZero() {
		return nil, apienv.NewError(400, apienv.CodePVCExpandUnsupported, "PVC storage request is missing", nil)
	}
	if target.Cmp(*currentStorage) <= 0 {
		return nil, apienv.NewError(400, apienv.CodePVCExpandNotIncreased, "Target capacity must be greater than current capacity", nil)
	}
	updated, err := s.kube.UpdatePVCStorageRequest(ctx, input.Namespace, input.Name, target)
	if err != nil {
		return nil, err
	}
	return s.pvcToDomain(ctx, updated)
}

func (s *ViewerService) ListStorageClasses(ctx context.Context) (items []domain.StorageClass, err error) {
	return NewStorageClassService(s.kube, s.recorder).ListStorageClasses(ctx, false)
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
		Namespace:  viewer.Namespace,
		PVCName:    pvc.Name,
		PVCUID:     string(pvc.UID),
		AccessMode: primaryAccessMode(accessModes),
		Mode:       viewerMode,
		MountInfo:  mountInfo,
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

func (s *ViewerService) detectPVCMounts(
	ctx context.Context,
	namespace string,
	pvcName string,
) (mountInfo *domain.PVCMountInfo, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
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

func (s *ViewerService) pvcToDomain(
	ctx context.Context,
	pvc *corev1.PersistentVolumeClaim,
) (*domain.PVC, error) {
	accessModes := make([]string, 0, len(pvc.Spec.AccessModes))
	for _, mode := range pvc.Spec.AccessModes {
		accessModes = append(accessModes, string(mode))
	}
	mountInfo, err := s.detectPVCMounts(ctx, pvc.Namespace, pvc.Name)
	if err != nil {
		return nil, err
	}
	supported, viewerMode, reason := kube.ViewerSupportForAccessModes(accessModes)
	return &domain.PVC{
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
	}, nil
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

func capacityQuantity(capacity string, capacityBytes int64) (resource.Quantity, error) {
	if strings.TrimSpace(capacity) != "" {
		quantity, err := resource.ParseQuantity(strings.TrimSpace(capacity))
		if err != nil {
			return resource.Quantity{}, err
		}
		if quantity.Sign() <= 0 {
			return resource.Quantity{}, apierrors.NewBadRequest("capacity must be positive")
		}
		return quantity, nil
	}
	if capacityBytes <= 0 {
		return resource.Quantity{}, apierrors.NewBadRequest("capacity must be positive")
	}
	return *resource.NewQuantity(capacityBytes, resource.BinarySI), nil
}

func persistentVolumeAccessModes(input []string) ([]corev1.PersistentVolumeAccessMode, error) {
	if len(input) == 0 {
		input = []string{domain.AccessModeReadWriteOnce}
	}
	accessModes := make([]corev1.PersistentVolumeAccessMode, 0, len(input))
	for _, mode := range input {
		switch strings.TrimSpace(mode) {
		case domain.AccessModeReadWriteOnce:
			accessModes = append(accessModes, corev1.ReadWriteOnce)
		case domain.AccessModeReadOnlyMany:
			accessModes = append(accessModes, corev1.ReadOnlyMany)
		case domain.AccessModeReadWriteMany:
			accessModes = append(accessModes, corev1.ReadWriteMany)
		default:
			return nil, apierrors.NewBadRequest("unsupported access mode")
		}
	}
	return accessModes, nil
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
