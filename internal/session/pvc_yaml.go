package session

import (
	"strings"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type PVCYAML struct {
	// Namespace is the Kubernetes namespace containing the PVC.
	Namespace string `json:"namespace"`
	// Name is the Kubernetes PersistentVolumeClaim name represented by the YAML.
	Name string `json:"name"`
	// YAML is an editable v1 PersistentVolumeClaim manifest with server-managed fields removed.
	YAML string `json:"yaml"`
}

type PVCDescribe struct {
	// Namespace is the Kubernetes namespace containing the PVC.
	Namespace string `json:"namespace"`
	// Name is the Kubernetes PersistentVolumeClaim name described by the text output.
	Name string `json:"name"`
	// Describe is kubectl-style diagnostic text for the PVC.
	Describe string `json:"describe"`
}

func parsePVCYAML(body string) (*corev1.PersistentVolumeClaim, error) {
	if strings.TrimSpace(body) == "" {
		return nil, apienv.NewError(400, apienv.CodeValidationError, "PVC YAML is required", nil)
	}
	var pvc corev1.PersistentVolumeClaim
	if err := yaml.Unmarshal([]byte(body), &pvc); err != nil {
		return nil, apienv.NewError(400, apienv.CodeValidationError, "PVC YAML is invalid", map[string]any{
			"error": err.Error(),
		})
	}
	if pvc.APIVersion == "" {
		pvc.APIVersion = "v1"
	}
	if pvc.Kind == "" {
		pvc.Kind = "PersistentVolumeClaim"
	}
	if pvc.APIVersion != "v1" || pvc.Kind != "PersistentVolumeClaim" {
		return nil, apienv.NewError(400, apienv.CodeValidationError, "YAML must define a v1 PersistentVolumeClaim", nil)
	}
	if strings.TrimSpace(pvc.Namespace) == "" {
		return nil, apienv.NewError(400, apienv.CodeValidationError, "PVC metadata.namespace is required", nil)
	}
	if strings.TrimSpace(pvc.Name) == "" {
		return nil, apienv.NewError(400, apienv.CodeValidationError, "PVC metadata.name is required", nil)
	}
	return &pvc, nil
}

func clearPVCServerFields(pvc *corev1.PersistentVolumeClaim) {
	pvc.UID = ""
	pvc.ResourceVersion = ""
	pvc.Generation = 0
	pvc.CreationTimestamp = metav1.Time{}
	pvc.ManagedFields = nil
	pvc.Status = corev1.PersistentVolumeClaimStatus{}
}
