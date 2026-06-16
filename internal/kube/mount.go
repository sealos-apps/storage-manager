package kube

import (
	"context"
	"slices"

	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
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

	info := DetectPVCMountsFromPods(pods)[pvcName]
	if info == nil {
		info = &domain.PVCMountInfo{
			MountedPods: []domain.MountedPod{},
			Nodes:       []string{},
		}
	}
	return info, nil
}

func DetectPVCMountsFromPods(pods []corev1.Pod) map[string]*domain.PVCMountInfo {
	mounts := map[string]*domain.PVCMountInfo{}
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil {
				continue
			}
			pvcName := volume.PersistentVolumeClaim.ClaimName
			if pvcName == "" {
				continue
			}
			info := mounts[pvcName]
			if info == nil {
				info = &domain.PVCMountInfo{
					MountedPods: []domain.MountedPod{},
					Nodes:       []string{},
				}
				mounts[pvcName] = info
			}
			mounted := domain.MountedPod{
				Namespace: pod.Namespace,
				Name:      pod.Name,
				NodeName:  pod.Spec.NodeName,
				Phase:     string(pod.Status.Phase),
				ReadOnly:  volume.PersistentVolumeClaim.ReadOnly,
			}
			info.MountedPods = append(info.MountedPods, mounted)
			if pod.Spec.NodeName != "" && !slices.Contains(info.Nodes, pod.Spec.NodeName) {
				info.Nodes = append(info.Nodes, pod.Spec.NodeName)
			}
		}
	}
	for _, info := range mounts {
		completeMountInfo(info)
	}
	return mounts
}

func completeMountInfo(info *domain.PVCMountInfo) {
	if info == nil {
		return
	}
	info.Mounted = len(info.MountedPods) > 0
	if len(info.Nodes) > 1 {
		info.Conflict = true
		info.Reason = "PVC_MOUNT_CONFLICT"
	}
	if len(info.Nodes) == 0 && len(info.MountedPods) > 0 {
		info.Reason = "PVC_MOUNT_PENDING"
	}
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
