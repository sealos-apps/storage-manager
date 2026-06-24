package session

import (
	"encoding/json"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	storagev1 "k8s.io/api/storage/v1"
)

func StorageClassToDomain(storageClass storagev1.StorageClass) domain.StorageClass {
	volumeBindingMode := ""
	if storageClass.VolumeBindingMode != nil {
		volumeBindingMode = string(*storageClass.VolumeBindingMode)
	}
	reclaimPolicy := ""
	if storageClass.ReclaimPolicy != nil {
		reclaimPolicy = string(*storageClass.ReclaimPolicy)
	}
	return domain.StorageClass{
		Name:                     storageClass.Name,
		Provisioner:              storageClass.Provisioner,
		AllowVolumeExpansion:     storageClass.AllowVolumeExpansion != nil && *storageClass.AllowVolumeExpansion,
		VolumeBindingMode:        volumeBindingMode,
		IsDefault:                storageClass.Annotations["storageclass.kubernetes.io/is-default-class"] == "true",
		ReclaimPolicy:            reclaimPolicy,
		CreationTimestampRFC3339: storageClass.CreationTimestamp.Format(time.RFC3339),
		ManagedByStorageManager:  storageClass.Labels[ManagedByLabel] == ManagedByValue,
		AvailableToUsers:         storageClass.Annotations[StorageClassAvailableToUsersAnnotation] == "true",
		DisplayNames:             storageClassDisplayNames(storageClass.Annotations),
	}
}

func storageClassDisplayNames(annotations map[string]string) map[string]string {
	var names map[string]string
	if err := json.Unmarshal([]byte(annotations[StorageClassDisplayNameAnnotation]), &names); err != nil {
		return map[string]string{}
	}
	if names == nil {
		return map[string]string{}
	}
	return names
}
