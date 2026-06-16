package session

import (
	"strings"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type StorageClassYAML struct {
	// Name is the Kubernetes StorageClass name represented by the YAML.
	Name string `json:"name"`
	// YAML is an editable storage.k8s.io/v1 StorageClass manifest with server-managed fields removed.
	YAML string `json:"yaml"`
}

type StorageClassDescribe struct {
	// Name is the Kubernetes StorageClass name described by the text output.
	Name string `json:"name"`
	// Describe is kubectl-style diagnostic text for the StorageClass.
	Describe string `json:"describe"`
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
