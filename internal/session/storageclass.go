package session

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/yaml"
)

type StorageClassService struct {
	kube     kube.Interface
	recorder *observability.Recorder
}

func NewStorageClassService(kubeClient kube.Interface, recorder *observability.Recorder) *StorageClassService {
	return &StorageClassService{
		kube:     kubeClient,
		recorder: recorder,
	}
}

func (s *StorageClassService) ListStorageClasses(ctx context.Context, includeHidden bool) (items []domain.StorageClass, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx, "storageclass.list", slog.Bool("include_hidden", includeHidden))
	defer func() {
		finish(err)
	}()

	storageClasses, err := s.kube.ListStorageClasses(ctx)
	if err != nil {
		return nil, err
	}
	usage := map[string]int{}
	if includeHidden {
		usage, err = s.storageClassUsage(ctx)
		if err != nil {
			return nil, err
		}
	}
	items = make([]domain.StorageClass, 0, len(storageClasses))
	for _, storageClass := range storageClasses {
		item := StorageClassToDomain(storageClass)
		item.InUsePVCCount = usage[item.Name]
		item.DeleteBlockedReason = storageClassDeleteBlockedReason(item)
		if !includeHidden && !item.VisibleInCreate {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *StorageClassService) GetStorageClassYAML(ctx context.Context, name string) (result *StorageClassYAML, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx, "storageclass.get_yaml", slog.String("storageclass", name))
	defer func() {
		finish(err)
	}()

	storageClass, err := s.kube.GetStorageClass(ctx, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodeStorageClassNotFound, "StorageClass not found", nil)
		}
		return nil, err
	}
	body, err := yaml.Marshal(storageClass)
	if err != nil {
		return nil, fmt.Errorf("marshaling storageclass yaml %s: %w", name, err)
	}
	return &StorageClassYAML{Name: storageClass.Name, YAML: string(body)}, nil
}

func (s *StorageClassService) CreateStorageClass(ctx context.Context, body string) (item *domain.StorageClass, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx, "storageclass.create")
	defer func() {
		finish(err)
	}()

	storageClass, err := parseStorageClassYAML(body)
	if err != nil {
		return nil, err
	}
	clearStorageClassServerFields(storageClass)
	if storageClass.Labels == nil {
		storageClass.Labels = map[string]string{}
	}
	storageClass.Labels[ManagedByLabel] = ManagedByValue
	created, err := s.kube.CreateStorageClass(ctx, storageClass)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, apienv.NewError(409, apienv.CodeStorageClassConflict, "StorageClass already exists", nil)
		}
		return nil, err
	}
	result := StorageClassToDomain(*created)
	return &result, nil
}

func (s *StorageClassService) UpdateStorageClass(
	ctx context.Context,
	name string,
	body string,
) (item *domain.StorageClass, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx, "storageclass.update", slog.String("storageclass", name))
	defer func() {
		finish(err)
	}()

	storageClass, err := parseStorageClassYAML(body)
	if err != nil {
		return nil, err
	}
	if storageClass.Name != name {
		return nil, apienv.NewError(400, apienv.CodeValidationError, "StorageClass YAML metadata.name must match path name", nil)
	}
	current, err := s.kube.GetStorageClass(ctx, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodeStorageClassNotFound, "StorageClass not found", nil)
		}
		return nil, err
	}
	if current.Labels[ManagedByLabel] == ManagedByValue {
		if storageClass.Labels == nil {
			storageClass.Labels = map[string]string{}
		}
		storageClass.Labels[ManagedByLabel] = ManagedByValue
	}
	updated, err := s.kube.UpdateStorageClass(ctx, storageClass)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodeStorageClassNotFound, "StorageClass not found", nil)
		}
		if apierrors.IsConflict(err) {
			return nil, apienv.NewError(409, apienv.CodeStorageClassConflict, "StorageClass was modified; reload YAML and retry", nil)
		}
		return nil, err
	}
	result := StorageClassToDomain(*updated)
	return &result, nil
}

func (s *StorageClassService) UpdateStorageClassPolicy(
	ctx context.Context,
	name string,
	input StorageClassPolicyInput,
) (item *domain.StorageClass, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx, "storageclass.update_policy", slog.String("storageclass", name))
	defer func() {
		finish(err)
	}()

	allowedModes, err := normalizeStorageClassAccessModes(input.AllowedAccessModes)
	if err != nil {
		return nil, err
	}
	current, err := s.kube.GetStorageClass(ctx, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodeStorageClassNotFound, "StorageClass not found", nil)
		}
		return nil, err
	}
	if current.Annotations == nil {
		current.Annotations = map[string]string{}
	}
	if input.VisibleInCreate {
		if len(allowedModes) == 0 {
			return nil, apienv.NewError(400, apienv.CodeValidationError, "StorageClass visible policy requires at least one access mode", nil)
		}
		current.Annotations[StorageClassVisibleAnnotation] = "true"
		current.Annotations[StorageClassAccessModesAnnotation] = strings.Join(allowedModes, ",")
	} else {
		current.Annotations[StorageClassVisibleAnnotation] = "false"
		delete(current.Annotations, StorageClassAccessModesAnnotation)
	}
	updated, err := s.kube.UpdateStorageClass(ctx, current)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodeStorageClassNotFound, "StorageClass not found", nil)
		}
		if apierrors.IsConflict(err) {
			return nil, apienv.NewError(409, apienv.CodeStorageClassConflict, "StorageClass was modified; reload and retry", nil)
		}
		return nil, err
	}
	result := StorageClassToDomain(*updated)
	return &result, nil
}

func (s *StorageClassService) DeleteStorageClass(ctx context.Context, name string) (item *domain.StorageClass, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx, "storageclass.delete", slog.String("storageclass", name))
	defer func() {
		finish(err)
	}()

	current, err := s.kube.GetStorageClass(ctx, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodeStorageClassNotFound, "StorageClass not found", nil)
		}
		return nil, err
	}
	deleted := StorageClassToDomain(*current)
	pvcCount, err := s.countStorageClassPVCs(ctx, name)
	if err != nil {
		return nil, err
	}
	deleted.InUsePVCCount = pvcCount
	deleted.DeleteBlockedReason = storageClassDeleteBlockedReason(deleted)
	if !deleted.ManagedByStorageManager {
		return nil, apienv.NewError(403, apienv.CodeStorageClassDeleteForbidden, "StorageClass was not created by this UI", map[string]any{
			"storage_class": name,
		})
	}
	if pvcCount > 0 {
		return nil, apienv.NewError(409, apienv.CodeStorageClassInUse, "StorageClass is used by existing PVCs", map[string]any{
			"storage_class": name,
			"pvc_count":     pvcCount,
		})
	}
	if err := s.kube.DeleteStorageClass(ctx, name); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodeStorageClassNotFound, "StorageClass not found", nil)
		}
		return nil, err
	}
	return &deleted, nil
}

func (s *StorageClassService) storageClassUsage(ctx context.Context) (map[string]int, error) {
	pvcs, err := s.kube.ListAllPVCs(ctx)
	if err != nil {
		return nil, err
	}
	usage := map[string]int{}
	for _, pvc := range pvcs {
		if pvc.Spec.StorageClassName == nil {
			continue
		}
		name := strings.TrimSpace(*pvc.Spec.StorageClassName)
		if name == "" {
			continue
		}
		usage[name]++
	}
	return usage, nil
}

func (s *StorageClassService) countStorageClassPVCs(ctx context.Context, name string) (int, error) {
	usage, err := s.storageClassUsage(ctx)
	if err != nil {
		return 0, err
	}
	return usage[name], nil
}

func storageClassDeleteBlockedReason(storageClass domain.StorageClass) string {
	if !storageClass.ManagedByStorageManager {
		return "not_managed"
	}
	if storageClass.InUsePVCCount > 0 {
		return "in_use"
	}
	return ""
}

func (s *StorageClassService) DescribeStorageClass(
	ctx context.Context,
	name string,
) (result *StorageClassDescribe, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx, "storageclass.describe", slog.String("storageclass", name))
	defer func() {
		finish(err)
	}()

	storageClass, err := s.kube.GetStorageClass(ctx, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodeStorageClassNotFound, "StorageClass not found", nil)
		}
		return nil, err
	}
	return &StorageClassDescribe{Name: storageClass.Name, Describe: describeStorageClass(*storageClass)}, nil
}
