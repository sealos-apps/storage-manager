package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestViewerServiceCreatePVC(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(&storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standard",
		},
		Provisioner: "example.test/provisioner",
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	pvc, err := service.CreatePVC(t.Context(), CreatePVCInput{
		Namespace:        "default",
		Name:             "data",
		Capacity:         "2Gi",
		AccessModes:      []string{domain.AccessModeReadWriteOnce},
		StorageClassName: "standard",
	})
	if err != nil {
		t.Fatalf("CreatePVC() error = %v", err)
	}
	if pvc.Name != "data" || pvc.Capacity != "2Gi" {
		t.Fatalf("pvc = %#v", pvc)
	}
	if pvc.StorageClassName != "standard" {
		t.Fatalf("storage class name = %q, want standard", pvc.StorageClassName)
	}
	created, err := clientset.CoreV1().PersistentVolumeClaims("default").Get(t.Context(), "data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() created pvc error = %v", err)
	}
	if created.Spec.StorageClassName == nil || *created.Spec.StorageClassName != "standard" {
		t.Fatalf("storage class = %#v", created.Spec.StorageClassName)
	}
}

func TestViewerServiceCreatePVCReturnsSuccessWhenReferenceScanFails(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(&storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standard",
		},
		Provisioner: "example.test/provisioner",
	})
	clientset.PrependReactor("list", "deployments", func(_ ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("reference scan unavailable")
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	pvc, err := service.CreatePVC(t.Context(), CreatePVCInput{
		Namespace:        "default",
		Name:             "data",
		Capacity:         "2Gi",
		AccessModes:      []string{domain.AccessModeReadWriteOnce},
		StorageClassName: "standard",
	})
	if err != nil {
		t.Fatalf("CreatePVC() error = %v", err)
	}
	if pvc.Name != "data" || len(pvc.References) != 0 {
		t.Fatalf("pvc = %#v", pvc)
	}
}

func TestViewerServicePVCYAMLAndDescribe(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	storageClassName := "standard"
	clientset := fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "data",
			UID:       types.UID("uid"),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storageClassName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	yamlResult, err := service.GetPVCYAML(t.Context(), "default", "data")
	if err != nil {
		t.Fatalf("GetPVCYAML() error = %v", err)
	}
	if !strings.Contains(yamlResult.YAML, "kind: PersistentVolumeClaim") || !strings.Contains(yamlResult.YAML, "namespace: default") {
		t.Fatalf("yaml = %s", yamlResult.YAML)
	}
	describe, err := service.DescribePVC(t.Context(), "default", "data")
	if err != nil {
		t.Fatalf("DescribePVC() error = %v", err)
	}
	for _, want := range []string{"Name: data", "Namespace: default", "StorageClass: standard", "Capacity: 1Gi"} {
		if !strings.Contains(describe.Describe, want) {
			t.Fatalf("describe missing %q: %s", want, describe.Describe)
		}
	}
	updatedBody := strings.Replace(yamlResult.YAML, "storage: 1Gi", "storage: 2Gi", 1)
	updated, err := service.UpdatePVC(t.Context(), "default", "data", updatedBody)
	if err != nil {
		t.Fatalf("UpdatePVC() error = %v", err)
	}
	if updated.Capacity != "2Gi" {
		t.Fatalf("updated capacity = %q", updated.Capacity)
	}
	_, err = service.UpdatePVC(t.Context(), "default", "other", updatedBody)
	var apiErr *apienv.Error
	if !errors.As(err, &apiErr) || apiErr.Code != apienv.CodeValidationError {
		t.Fatalf("UpdatePVC(mismatch) error = %#v", err)
	}
}

func TestViewerServiceListPVCsIncludesStorageClassName(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	storageClassName := "standard"
	client := kube.New(fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: &storageClassName,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "empty", UID: types.UID("empty-uid")},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
	))
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	items, err := service.ListPVCs(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListPVCs() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	byName := map[string]domain.PVC{}
	for _, item := range items {
		byName[item.Name] = item
	}
	if byName["data"].StorageClassName != "standard" {
		t.Fatalf("data storage class name = %q, want standard", byName["data"].StorageClassName)
	}
	if byName["empty"].StorageClassName != "" {
		t.Fatalf("empty storage class name = %q, want empty", byName["empty"].StorageClassName)
	}
}

func TestViewerServiceListPVCsIncludesReferences(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := kube.New(fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "demo",
				Labels: map[string]string{
					kube.PVCReferenceSourceLabel:     "true",
					kube.PVCReferenceSourceTypeLabel: "applaunchpad",
				},
				Annotations: map[string]string{
					kube.PVCReferenceNameAnnotation: "Demo App",
					kube.PVCReferencesAnnotation:    `[{"name":"data","mountPath":"/data"}]`,
				},
			},
		},
	))
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	items, err := service.ListPVCs(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListPVCs() error = %v", err)
	}
	if len(items) != 1 || len(items[0].References) != 1 {
		t.Fatalf("items = %#v", items)
	}
	reference := items[0].References[0]
	if reference.SourceType != "applaunchpad" ||
		reference.SourceName != "Demo App" ||
		reference.MountPath != "/data" {
		t.Fatalf("reference = %#v", reference)
	}
}

func TestViewerServiceCreatePVCRejectsUnsupportedAccessMode(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := kube.New(fake.NewSimpleClientset(&storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standard",
		},
		Provisioner: "example.test/provisioner",
	}))
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	_, err := service.CreatePVC(t.Context(), CreatePVCInput{
		Namespace:        "default",
		Name:             "data",
		Capacity:         "2Gi",
		AccessModes:      []string{domain.AccessModeReadWriteOncePod},
		StorageClassName: "standard",
	})

	var apiErr *apienv.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("CreatePVC() error = %T %v, want apienv.Error", err, err)
	}
	if apiErr.Code != apienv.CodeUnsupportedAccessMode {
		t.Fatalf("code = %s, want %s", apiErr.Code, apienv.CodeUnsupportedAccessMode)
	}
}

func TestViewerServiceDeletePVCRejectsMountedPVC(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := kube.New(fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
		testMountedPod("default", "app", "node-a", "data"),
	))
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	if _, err := service.DeletePVC(t.Context(), DeletePVCInput{Namespace: "default", Name: "data"}); err == nil {
		t.Fatal("DeletePVC() error = nil")
	}
}

func TestViewerServiceDeletePVCRejectsReferencedPVC(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "demo",
				Labels: map[string]string{
					kube.PVCReferenceSourceLabel:     "true",
					kube.PVCReferenceSourceTypeLabel: "applaunchpad",
				},
				Annotations: map[string]string{
					kube.PVCReferencesAnnotation: `[{"name":"data"}]`,
				},
			},
		},
	)
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	_, err := service.DeletePVC(t.Context(), DeletePVCInput{Namespace: "default", Name: "data"})
	var apiErr *apienv.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("DeletePVC() error = %T %v, want apienv.Error", err, err)
	}
	if apiErr.Code != apienv.CodePVCReferenced {
		t.Fatalf("code = %s, want %s", apiErr.Code, apienv.CodePVCReferenced)
	}
	if _, err := clientset.CoreV1().PersistentVolumeClaims("default").Get(t.Context(), "data", metav1.GetOptions{}); err != nil {
		t.Fatalf("referenced pvc was deleted: %v", err)
	}
}

func TestViewerServiceDeletePVCFailsClosedWhenReferenceScanFails(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "demo",
				Labels: map[string]string{
					kube.PVCReferenceSourceLabel:     "true",
					kube.PVCReferenceSourceTypeLabel: "applaunchpad",
				},
				Annotations: map[string]string{
					kube.PVCReferencesAnnotation: `not-json`,
				},
			},
		},
	)
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	if _, err := service.DeletePVC(t.Context(), DeletePVCInput{Namespace: "default", Name: "data"}); err == nil {
		t.Fatal("DeletePVC() error = nil")
	}
	if _, err := clientset.CoreV1().PersistentVolumeClaims("default").Get(t.Context(), "data", metav1.GetOptions{}); err != nil {
		t.Fatalf("pvc was deleted after reference scan failure: %v", err)
	}
}

func TestViewerServiceDeletePVCUsesCheckedReferenceState(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
	)
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	deleted, err := service.DeletePVC(t.Context(), DeletePVCInput{Namespace: "default", Name: "data"})
	if err != nil {
		t.Fatalf("DeletePVC() error = %v", err)
	}
	if len(deleted.References) != 0 {
		t.Fatalf("deleted references = %#v, want none", deleted.References)
	}

	if got := countListActions(clientset.Actions(), "deployments"); got != 1 {
		t.Fatalf("deployment reference scans = %d, want 1", got)
	}
	if got := countListActions(clientset.Actions(), "statefulsets"); got != 1 {
		t.Fatalf("statefulset reference scans = %d, want 1", got)
	}
}

func TestViewerServiceExpandPVC(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "data", UID: types.UID("uid")},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	pods := NewPodService(cfg, store, client, observability.MustNew(cfg.Observability, nil))
	service := NewViewerService(cfg, store, client, pods, nil, observability.MustNew(cfg.Observability, nil))

	pvc, err := service.ExpandPVC(t.Context(), ExpandPVCInput{
		Namespace: "default",
		Name:      "data",
		Capacity:  "3Gi",
	})
	if err != nil {
		t.Fatalf("ExpandPVC() error = %v", err)
	}
	if pvc.Capacity != "3Gi" {
		t.Fatalf("pvc = %#v", pvc)
	}
	updated, err := clientset.CoreV1().PersistentVolumeClaims("default").Get(t.Context(), "data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() updated pvc error = %v", err)
	}
	if updated.Spec.Resources.Requests.Storage().String() != "3Gi" {
		t.Fatalf("storage = %s", updated.Spec.Resources.Requests.Storage().String())
	}
}

func TestViewerServiceExpandPVCDeniesPendingPVC(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "pending", UID: types.UID("pending-uid")},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	recorder := observability.MustNew(cfg.Observability, nil)
	service := NewViewerService(cfg, store, client, NewPodService(cfg, store, client, recorder), nil, recorder)

	_, err := service.ExpandPVC(t.Context(), ExpandPVCInput{
		Namespace: "default",
		Name:      "pending",
		Capacity:  "20Gi",
	})
	if err == nil {
		t.Fatal("ExpandPVC() error = nil, want pending PVC error")
	}
	apiErr, ok := err.(*apienv.Error)
	if !ok || apiErr.Code != apienv.CodePVCExpandPending || apiErr.Status != 400 {
		t.Fatalf("ExpandPVC() error = %#v, want PVC_EXPAND_PENDING 400", err)
	}
	if apiErr.Details["phase"] != corev1.ClaimPending {
		t.Fatalf("pending phase detail = %#v", apiErr.Details["phase"])
	}
}

func TestViewerServiceExpandPVCDeniesLostPVC(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "lost", UID: types.UID("lost-uid")},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimLost},
	})
	client := kube.New(clientset)
	store := state.New(cfg.Cache)
	recorder := observability.MustNew(cfg.Observability, nil)
	service := NewViewerService(cfg, store, client, NewPodService(cfg, store, client, recorder), nil, recorder)

	_, err := service.ExpandPVC(t.Context(), ExpandPVCInput{
		Namespace: "default",
		Name:      "lost",
		Capacity:  "20Gi",
	})
	if err == nil {
		t.Fatal("ExpandPVC() error = nil, want lost PVC error")
	}
	apiErr, ok := err.(*apienv.Error)
	if !ok || apiErr.Code != apienv.CodePVCExpandLost || apiErr.Status != 400 {
		t.Fatalf("ExpandPVC() error = %#v, want PVC_EXPAND_LOST 400", err)
	}
	if apiErr.Details["phase"] != corev1.ClaimLost {
		t.Fatalf("lost phase detail = %#v", apiErr.Details["phase"])
	}
}

func countListActions(actions []ktesting.Action, resource string) int {
	count := 0
	for _, action := range actions {
		if action.GetVerb() == "list" && action.GetResource().Resource == resource {
			count++
		}
	}
	return count
}
