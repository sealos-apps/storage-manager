package session

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

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

func (s *ViewerService) ListNamespaces(ctx context.Context) (items []corev1.Namespace, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx, "viewer.list_namespaces")
	defer func() {
		finish(err)
	}()
	return s.kube.ListNamespaces(ctx)
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
	volumeStats := s.listPVCVolumeStats(ctx, namespace)
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
		capacityBytes := pvc.Spec.Resources.Requests.Storage().Value()
		items = append(items, domain.PVC{
			Namespace:        pvc.Namespace,
			Name:             pvc.Name,
			UID:              string(pvc.UID),
			CapacityBytes:    capacityBytes,
			Capacity:         pvc.Spec.Resources.Requests.Storage().String(),
			AccessModes:      accessModes,
			StorageClassName: pvcStorageClassName(&pvc),
			Mounted:          mountInfo.Mounted,
			MountedPods:      mountInfo.MountedPods,
			ViewerSupported:  supported,
			ViewerMode:       viewerMode,
			ViewerScheduling: kube.SchedulingForPVC(accessModes, mountInfo),
			Reason:           reason,
			VolumeStats:      normalizedPVCVolumeStats(capacityBytes, volumeStats[pvc.Name]),
		})
	}
	s.recorder.Logger().LogAttrs(ctx, slog.LevelDebug, "viewer.list_pvcs.result",
		slog.String("namespace", namespace),
		slog.Int("pvc_count", len(items)),
	)
	return items, nil
}

func (s *ViewerService) listPVCVolumeStats(ctx context.Context, namespace string) map[string]domain.PVCVolumeStats {
	if s.pvcMetrics == nil {
		return nil
	}
	stats, err := s.pvcMetrics.ListPVCVolumeStats(ctx, namespace)
	if err != nil {
		s.recorder.Logger().LogAttrs(ctx, slog.LevelWarn, "pvc.metrics_unavailable",
			slog.String("namespace", namespace),
			slog.String("error", err.Error()),
		)
		return nil
	}
	return stats
}

func normalizedPVCVolumeStats(capacityBytes int64, stats domain.PVCVolumeStats) *domain.PVCVolumeStats {
	if stats.Source == "" || stats.UsedBytes == 0 && stats.MetricCapacityBytes == 0 && stats.AvailableBytes == 0 {
		return nil
	}
	if !pvcVolumeStatsMatchClaim(capacityBytes, stats) {
		stats.Status = "mismatched"
	}
	return &stats
}

func pvcVolumeStatsMatchClaim(capacityBytes int64, stats domain.PVCVolumeStats) bool {
	if capacityBytes <= 0 || stats.MetricCapacityBytes <= 0 || stats.UsedBytes < 0 || stats.AvailableBytes < 0 {
		return false
	}
	if stats.UsedBytes > capacityBytes {
		return false
	}
	delta := stats.MetricCapacityBytes - capacityBytes
	if delta < 0 {
		delta = -delta
	}
	return delta*100 <= capacityBytes*10
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
				ManagedByLabel: ManagedByValue,
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
	_, err = s.kube.GetStorageClass(ctx, storageClassName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodeStorageClassNotFound, "StorageClass not found", nil)
		}
		return nil, err
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
		StorageClassName: pvcStorageClassName(pvc),
		Mounted:          mountInfo.Mounted,
		MountedPods:      mountInfo.MountedPods,
		ViewerSupported:  supported,
		ViewerMode:       viewerMode,
		ViewerScheduling: kube.SchedulingForPVC(accessModes, mountInfo),
		Reason:           reason,
	}, nil
}

func pvcStorageClassName(pvc *corev1.PersistentVolumeClaim) string {
	if pvc.Spec.StorageClassName == nil {
		return ""
	}
	return strings.TrimSpace(*pvc.Spec.StorageClassName)
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
