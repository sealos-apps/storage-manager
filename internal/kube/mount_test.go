package kube

import (
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDetectPVCMounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pods       []corev1.Pod
		wantNodes  []string
		wantPods   int
		wantReason string
	}{
		{
			name: "running pod on one node",
			pods: []corev1.Pod{
				testPod("default", "app-0", "node-a", corev1.PodRunning, "data", false),
			},
			wantNodes: []string{"node-a"},
			wantPods:  1,
		},
		{
			name: "failed pod ignored",
			pods: []corev1.Pod{
				testPod("default", "done", "node-a", corev1.PodFailed, "data", false),
			},
			wantNodes: []string{},
			wantPods:  0,
		},
		{
			name: "multi node conflict",
			pods: []corev1.Pod{
				testPod("default", "app-0", "node-a", corev1.PodRunning, "data", false),
				testPod("default", "app-1", "node-b", corev1.PodRunning, "data", false),
			},
			wantNodes:  []string{"node-a", "node-b"},
			wantPods:   2,
			wantReason: "PVC_MOUNT_CONFLICT",
		},
		{
			name: "pending without node",
			pods: []corev1.Pod{
				testPod("default", "pending", "", corev1.PodPending, "data", true),
			},
			wantNodes:  []string{},
			wantPods:   1,
			wantReason: "PVC_MOUNT_PENDING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(fake.NewSimpleClientset(podObjects(tt.pods)...))
			detector := NewPVCMountDetector(client)
			info, err := detector.DetectPVCMounts(t.Context(), "default", "data")
			if err != nil {
				t.Fatalf("DetectPVCMounts() error = %v", err)
			}
			if len(info.Nodes) != len(tt.wantNodes) {
				t.Fatalf("nodes = %#v, want %#v", info.Nodes, tt.wantNodes)
			}
			if len(info.MountedPods) != tt.wantPods {
				t.Fatalf("mounted pods = %d, want %d", len(info.MountedPods), tt.wantPods)
			}
			if info.Reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", info.Reason, tt.wantReason)
			}
		})
	}
}

func TestViewerSupportForAccessModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		accessModes   []string
		wantSupported bool
		wantMode      string
	}{
		{
			name:          "rwx",
			accessModes:   []string{domain.AccessModeReadWriteMany},
			wantSupported: true,
			wantMode:      domain.ModeReadWrite,
		},
		{
			name:          "rox",
			accessModes:   []string{domain.AccessModeReadOnlyMany},
			wantSupported: true,
			wantMode:      domain.ModeReadOnly,
		},
		{
			name:          "rwop",
			accessModes:   []string{domain.AccessModeReadWriteOncePod},
			wantSupported: false,
			wantMode:      "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSupported, gotMode, _ := ViewerSupportForAccessModes(tt.accessModes)
			if gotSupported != tt.wantSupported || gotMode != tt.wantMode {
				t.Fatalf("got supported=%v mode=%q", gotSupported, gotMode)
			}
		})
	}
}

func TestSchedulingForPVC(t *testing.T) {
	t.Parallel()

	info := &domain.PVCMountInfo{
		Mounted: true,
		Nodes:   []string{"node-a"},
	}
	scheduling := SchedulingForPVC([]string{domain.AccessModeReadWriteOnce}, info)
	if !scheduling.RequiresNode || scheduling.NodeName != "node-a" {
		t.Fatalf("scheduling = %#v", scheduling)
	}
}

func testPod(
	namespace string,
	name string,
	node string,
	phase corev1.PodPhase,
	pvc string,
	readOnly bool,
) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: corev1.PodSpec{
			NodeName: node,
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc,
							ReadOnly:  readOnly,
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}

func podObjects(pods []corev1.Pod) []runtime.Object {
	objects := make([]runtime.Object, 0, len(pods))
	for i := range pods {
		objects = append(objects, new(pods[i]))
	}
	return objects
}
