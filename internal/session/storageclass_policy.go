package session

import (
	"slices"
	"strings"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
)

const (
	ManagedByLabel = "app.kubernetes.io/managed-by"
	ManagedByValue = "sealos-storage-manager"

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

type StorageClassPolicyInput struct {
	AllowedAccessModes []string
	VisibleInCreate    bool
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
