package session

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const (
	StorageClassVisibleAnnotation     = "storage-management.sealos.io/visible-in-create"
	StorageClassAccessModesAnnotation = "storage-management.sealos.io/access-modes"

	storageClassAnnotationReady   = "ready"
	storageClassAnnotationHidden  = "hidden"
	storageClassAnnotationInvalid = "invalid"
)

var supportedStorageClassAccessModes = []string{
	domain.AccessModeReadWriteOnce,
	domain.AccessModeReadOnlyMany,
	domain.AccessModeReadWriteMany,
}

type StorageClassService struct {
	kube     kube.Interface
	recorder *observability.Recorder
}

type StorageClassYAML struct {
	Name string `json:"name"`
	YAML string `json:"yaml"`
}

type StorageClassDescribe struct {
	Name     string `json:"name"`
	Describe string `json:"describe"`
}

type StorageClassPolicyInput struct {
	AllowedAccessModes []string
	VisibleInCreate    bool
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
	items = make([]domain.StorageClass, 0, len(storageClasses))
	for _, storageClass := range storageClasses {
		item := StorageClassToDomain(storageClass)
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
	if err := s.kube.DeleteStorageClass(ctx, name); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apienv.NewError(404, apienv.CodeStorageClassNotFound, "StorageClass not found", nil)
		}
		return nil, err
	}
	return &deleted, nil
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

func StorageClassToDomain(storageClass storagev1.StorageClass) domain.StorageClass {
	volumeBindingMode := ""
	if storageClass.VolumeBindingMode != nil {
		volumeBindingMode = string(*storageClass.VolumeBindingMode)
	}
	reclaimPolicy := ""
	if storageClass.ReclaimPolicy != nil {
		reclaimPolicy = string(*storageClass.ReclaimPolicy)
	}
	allowedModes, status := StorageClassAccessPolicy(storageClass.Annotations)
	return domain.StorageClass{
		Name:                     storageClass.Name,
		Provisioner:              storageClass.Provisioner,
		AllowVolumeExpansion:     storageClass.AllowVolumeExpansion != nil && *storageClass.AllowVolumeExpansion,
		VolumeBindingMode:        volumeBindingMode,
		IsDefault:                storageClass.Annotations["storageclass.kubernetes.io/is-default-class"] == "true",
		ReclaimPolicy:            reclaimPolicy,
		CreationTimestampRFC3339: storageClass.CreationTimestamp.Format(time.RFC3339),
		VisibleInCreate:          status == storageClassAnnotationReady,
		AllowedAccessModes:       allowedModes,
		AnnotationStatus:         status,
	}
}

func StorageClassAccessPolicy(annotations map[string]string) ([]string, string) {
	if annotations[StorageClassVisibleAnnotation] != "true" {
		return []string{}, storageClassAnnotationHidden
	}
	modes, err := normalizeStorageClassAccessModes(strings.Split(annotations[StorageClassAccessModesAnnotation], ","))
	if err != nil {
		return []string{}, storageClassAnnotationInvalid
	}
	if len(modes) == 0 {
		return []string{}, storageClassAnnotationInvalid
	}
	return modes, storageClassAnnotationReady
}

func normalizeStorageClassAccessModes(rawModes []string) ([]string, error) {
	seen := map[string]struct{}{}
	var modes []string
	for _, raw := range rawModes {
		mode := strings.TrimSpace(raw)
		if mode == "" {
			continue
		}
		if !slices.Contains(supportedStorageClassAccessModes, mode) {
			return nil, apienv.NewError(400, apienv.CodeValidationError, "unsupported StorageClass access mode", map[string]any{
				"access_mode": mode,
			})
		}
		if _, ok := seen[mode]; ok {
			continue
		}
		seen[mode] = struct{}{}
		modes = append(modes, mode)
	}
	return modes, nil
}

func parseStorageClassYAML(body string) (*storagev1.StorageClass, error) {
	if strings.TrimSpace(body) == "" {
		return nil, apienv.NewError(400, apienv.CodeStorageClassYAMLInvalid, "StorageClass YAML is required", nil)
	}
	var storageClass storagev1.StorageClass
	if err := yaml.Unmarshal([]byte(body), &storageClass); err != nil {
		return nil, apienv.NewError(400, apienv.CodeStorageClassYAMLInvalid, "StorageClass YAML is invalid", map[string]any{
			"error": err.Error(),
		})
	}
	if storageClass.APIVersion == "" {
		storageClass.APIVersion = "storage.k8s.io/v1"
	}
	if storageClass.Kind == "" {
		storageClass.Kind = "StorageClass"
	}
	if storageClass.APIVersion != "storage.k8s.io/v1" || storageClass.Kind != "StorageClass" {
		return nil, apienv.NewError(400, apienv.CodeStorageClassYAMLInvalid, "YAML must define a storage.k8s.io/v1 StorageClass", nil)
	}
	if strings.TrimSpace(storageClass.Name) == "" {
		return nil, apienv.NewError(400, apienv.CodeValidationError, "StorageClass metadata.name is required", nil)
	}
	if strings.TrimSpace(storageClass.Namespace) != "" {
		return nil, apienv.NewError(400, apienv.CodeValidationError, "StorageClass metadata.namespace must be empty", nil)
	}
	return &storageClass, nil
}

func clearStorageClassServerFields(storageClass *storagev1.StorageClass) {
	storageClass.UID = ""
	storageClass.ResourceVersion = ""
	storageClass.Generation = 0
	storageClass.CreationTimestamp = metav1.Time{}
	storageClass.ManagedFields = nil
}

func describeStorageClass(storageClass storagev1.StorageClass) string {
	item := StorageClassToDomain(storageClass)
	lines := []string{
		"Name: " + storageClass.Name,
		"Provisioner: " + storageClass.Provisioner,
		"Default: " + boolText(item.IsDefault),
		"Visible In Create: " + boolText(item.VisibleInCreate),
		"Annotation Status: " + item.AnnotationStatus,
		"Allowed Access Modes: " + strings.Join(item.AllowedAccessModes, ", "),
		"Reclaim Policy: " + item.ReclaimPolicy,
		"Volume Binding Mode: " + item.VolumeBindingMode,
		"Allow Volume Expansion: " + boolText(item.AllowVolumeExpansion),
		"Creation Timestamp: " + item.CreationTimestampRFC3339,
		"",
		"Parameters:",
	}
	lines = appendMap(lines, storageClass.Parameters)
	lines = append(lines, "", "Mount Options:")
	if len(storageClass.MountOptions) == 0 {
		lines = append(lines, "  <none>")
	} else {
		for _, option := range storageClass.MountOptions {
			lines = append(lines, "  - "+option)
		}
	}
	lines = append(lines, "", "Annotations:")
	lines = appendMap(lines, storageClass.Annotations)
	return strings.Join(lines, "\n")
}

func appendMap(lines []string, values map[string]string) []string {
	if len(values) == 0 {
		return append(lines, "  <none>")
	}
	cloned := maps.Clone(values)
	keys := make([]string, 0, len(cloned))
	for key := range cloned {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, "  "+key+": "+cloned[key])
	}
	return lines
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
