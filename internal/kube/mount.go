package kube

import (
	"context"
	"slices"

	"github.com/nixieboluo/sealos-stroage-manager/internal/domain"
	corev1 "k8s.io/api/core/v1"
)

type PVCMountDetector struct {
	client Interface
}

func NewPVCMountDetector(client Interface) *PVCMountDetector {
	return &PVCMountDetector{client: client}
}

func (d *PVCMountDetector) DetectPVCMounts(
	ctx context.Context,
	namespace string,
	pvcName string,
) (*domain.PVCMountInfo, error) {
	pods, err := d.client.ListPods(ctx, namespace)
	if err != nil {
		return nil, err
	}

	info := &domain.PVCMountInfo{
		MountedPods: []domain.MountedPod{},
		Nodes:       []string{},
	}
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		readOnly, ok := podUsesPVC(pod, pvcName)
		if !ok {
			continue
		}
		mounted := domain.MountedPod{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			NodeName:  pod.Spec.NodeName,
			Phase:     string(pod.Status.Phase),
			ReadOnly:  readOnly,
		}
		info.MountedPods = append(info.MountedPods, mounted)
		if pod.Spec.NodeName != "" && !slices.Contains(info.Nodes, pod.Spec.NodeName) {
			info.Nodes = append(info.Nodes, pod.Spec.NodeName)
		}
	}
	info.Mounted = len(info.MountedPods) > 0
	if len(info.Nodes) > 1 {
		info.Conflict = true
		info.Reason = "PVC_MOUNT_CONFLICT"
	}
	if len(info.Nodes) == 0 && len(info.MountedPods) > 0 {
		info.Reason = "PVC_MOUNT_PENDING"
	}
	return info, nil
}

func podUsesPVC(pod corev1.Pod, pvcName string) (bool, bool) {
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName != pvcName {
			continue
		}
		return volume.PersistentVolumeClaim.ReadOnly, true
	}
	return false, false
}

func SchedulingForPVC(accessModes []string, mountInfo *domain.PVCMountInfo) domain.ViewerScheduling {
	if !slices.Contains(accessModes, domain.AccessModeReadWriteOnce) {
		return domain.ViewerScheduling{}
	}
	if mountInfo == nil || !mountInfo.Mounted {
		return domain.ViewerScheduling{}
	}
	if mountInfo.Conflict {
		return domain.ViewerScheduling{Reason: domain.AccessModeReadWriteOnce + " PVC is mounted on multiple nodes"}
	}
	if mountInfo.Reason == "PVC_MOUNT_PENDING" {
		return domain.ViewerScheduling{Reason: "ReadWriteOnce PVC is referenced by pending pods"}
	}
	if len(mountInfo.Nodes) == 1 {
		return domain.ViewerScheduling{
			RequiresNode: true,
			NodeName:     mountInfo.Nodes[0],
			Reason:       "ReadWriteOnce PVC is already mounted on " + mountInfo.Nodes[0],
		}
	}
	return domain.ViewerScheduling{}
}

func ViewerSupportForAccessModes(accessModes []string) (supported bool, mode string, reason string) {
	switch {
	case slices.Contains(accessModes, domain.AccessModeReadWriteOncePod):
		return false, "", "ReadWriteOncePod is not supported"
	case slices.Contains(accessModes, domain.AccessModeReadOnlyMany):
		return true, domain.ModeReadOnly, ""
	case slices.Contains(accessModes, domain.AccessModeReadWriteMany):
		return true, domain.ModeReadWrite, ""
	case slices.Contains(accessModes, domain.AccessModeReadWriteOnce):
		return true, domain.ModeReadWrite, ""
	default:
		return false, "", "PVC access mode is not supported"
	}
}
