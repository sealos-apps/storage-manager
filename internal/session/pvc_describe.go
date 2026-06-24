package session

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func describePVC(pvc corev1.PersistentVolumeClaim) string {
	lines := []string{
		"Name: " + pvc.Name,
		"Namespace: " + pvc.Namespace,
		"StorageClass: " + pvcStorageClassName(&pvc),
		"Status: " + string(pvc.Status.Phase),
		"Volume: " + pvc.Spec.VolumeName,
		"Capacity: " + pvc.Spec.Resources.Requests.Storage().String(),
		"",
		"Access Modes:",
	}
	if len(pvc.Spec.AccessModes) == 0 {
		lines = append(lines, "  <none>")
	} else {
		for _, mode := range pvc.Spec.AccessModes {
			lines = append(lines, "  - "+string(mode))
		}
	}
	lines = append(lines, "", "Labels:")
	lines = appendMap(lines, pvc.Labels)
	lines = append(lines, "", "Annotations:")
	lines = appendMap(lines, pvc.Annotations)
	return strings.Join(lines, "\n")
}
