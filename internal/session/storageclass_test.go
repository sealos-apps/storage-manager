package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStorageClassServiceCRUDYAMLAndDescribe(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset()
	service := NewStorageClassService(
		kube.New(clientset),
		observability.MustNew(cfg.Observability, nil),
	)
	body := `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: fast
provisioner: example.test/provisioner
`

	created, err := service.CreateStorageClass(t.Context(), body)
	if err != nil {
		t.Fatalf("CreateStorageClass() error = %v", err)
	}
	if !created.ManagedByStorageManager {
		t.Fatalf("created managed flag = false: %#v", created)
	}
	createdKube, err := clientset.StorageV1().StorageClasses().Get(t.Context(), "fast", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get created storageclass: %v", err)
	}
	if createdKube.Labels[ManagedByLabel] != ManagedByValue {
		t.Fatalf("managed label = %#v", createdKube.Labels)
	}
	yamlResult, err := service.GetStorageClassYAML(t.Context(), "fast")
	if err != nil {
		t.Fatalf("GetStorageClassYAML() error = %v", err)
	}
	if !strings.Contains(yamlResult.YAML, "name: fast") {
		t.Fatalf("yaml = %s", yamlResult.YAML)
	}
	describe, err := service.DescribeStorageClass(t.Context(), "fast")
	if err != nil {
		t.Fatalf("DescribeStorageClass() error = %v", err)
	}
	if strings.Contains(describe.Describe, "Visible In Create") ||
		strings.Contains(describe.Describe, "Annotation Status") ||
		strings.Contains(describe.Describe, "Allowed Access Modes") {
		t.Fatalf("describe = %s", describe.Describe)
	}
	updated, err := service.UpdateStorageClass(t.Context(), "fast", strings.Replace(body, "example.test/provisioner", "example.test/updated", 1))
	if err != nil {
		t.Fatalf("UpdateStorageClass() error = %v", err)
	}
	if updated.Provisioner != "example.test/updated" {
		t.Fatalf("updated = %#v", updated)
	}
	deleted, err := service.DeleteStorageClass(t.Context(), "fast")
	if err != nil {
		t.Fatalf("DeleteStorageClass() error = %v", err)
	}
	if deleted.Name != "fast" {
		t.Fatalf("deleted = %#v", deleted)
	}
}

func TestStorageClassServiceDeleteRequiresManagedLabel(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	service := NewStorageClassService(
		kube.New(fake.NewSimpleClientset(&storagev1.StorageClass{
			ObjectMeta:  metav1.ObjectMeta{Name: "external"},
			Provisioner: "example.test/provisioner",
		})),
		observability.MustNew(cfg.Observability, nil),
	)

	_, err := service.DeleteStorageClass(t.Context(), "external")

	var apiErr *apienv.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("DeleteStorageClass() error = %T %v, want apienv.Error", err, err)
	}
	if apiErr.Code != apienv.CodeStorageClassDeleteForbidden {
		t.Fatalf("code = %s, want %s", apiErr.Code, apienv.CodeStorageClassDeleteForbidden)
	}
}

func TestStorageClassServiceUpdateMetadata(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	clientset := fake.NewSimpleClientset(&storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standard",
		},
		Provisioner: "example.test/provisioner",
	})
	service := NewStorageClassService(
		kube.New(clientset),
		observability.MustNew(cfg.Observability, nil),
	)

	updated, err := service.UpdateStorageClassMetadata(t.Context(), "standard", StorageClassMetadataInput{
		AvailableToUsers: true,
		DisplayNames: map[string]string{
			" zh ": " 高性能云盘 ",
			"en":   "Fast Disk",
			"":     "ignored",
		},
	})
	if err != nil {
		t.Fatalf("UpdateStorageClassMetadata() error = %v", err)
	}
	if !updated.AvailableToUsers || updated.DisplayNames["zh"] != "高性能云盘" || updated.DisplayNames["en"] != "Fast Disk" {
		t.Fatalf("updated = %#v", updated)
	}
	kubeStorageClass, err := clientset.StorageV1().StorageClasses().Get(t.Context(), "standard", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get storageclass: %v", err)
	}
	if kubeStorageClass.Annotations[StorageClassAvailableToUsersAnnotation] != "true" {
		t.Fatalf("annotations = %#v", kubeStorageClass.Annotations)
	}
	if !strings.Contains(kubeStorageClass.Annotations[StorageClassDisplayNameAnnotation], `"zh":"高性能云盘"`) {
		t.Fatalf("display names annotation = %q", kubeStorageClass.Annotations[StorageClassDisplayNameAnnotation])
	}

	updated, err = service.UpdateStorageClassMetadata(t.Context(), "standard", StorageClassMetadataInput{})
	if err != nil {
		t.Fatalf("UpdateStorageClassMetadata(empty) error = %v", err)
	}
	if updated.AvailableToUsers || len(updated.DisplayNames) != 0 {
		t.Fatalf("empty updated = %#v", updated)
	}
	kubeStorageClass, err = clientset.StorageV1().StorageClasses().Get(t.Context(), "standard", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get storageclass: %v", err)
	}
	if _, ok := kubeStorageClass.Annotations[StorageClassDisplayNameAnnotation]; ok {
		t.Fatalf("display names annotation kept: %#v", kubeStorageClass.Annotations)
	}
	if kubeStorageClass.Annotations[StorageClassAvailableToUsersAnnotation] != "false" {
		t.Fatalf("annotations = %#v", kubeStorageClass.Annotations)
	}
}

func TestStorageClassServiceDeleteRejectsStorageClassInUseByAnyPVC(t *testing.T) {
	t.Parallel()

	storageClassName := "fast"
	cfg := testConfig()
	service := NewStorageClassService(
		kube.New(fake.NewSimpleClientset(
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: storageClassName,
					Labels: map[string]string{
						ManagedByLabel: ManagedByValue,
					},
				},
				Provisioner: "example.test/provisioner",
			},
			&corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "other",
					Name:      "data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &storageClassName,
				},
			},
		)),
		observability.MustNew(cfg.Observability, nil),
	)

	_, err := service.DeleteStorageClass(t.Context(), storageClassName)

	var apiErr *apienv.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("DeleteStorageClass() error = %T %v, want apienv.Error", err, err)
	}
	if apiErr.Code != apienv.CodeStorageClassInUse {
		t.Fatalf("code = %s, want %s", apiErr.Code, apienv.CodeStorageClassInUse)
	}
	if apiErr.Details["pvc_count"] != 1 {
		t.Fatalf("details = %#v", apiErr.Details)
	}
}

func TestStorageClassServiceAdminListIncludesDeleteMetadata(t *testing.T) {
	t.Parallel()

	storageClassName := "fast"
	cfg := testConfig()
	service := NewStorageClassService(
		kube.New(fake.NewSimpleClientset(
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: storageClassName,
					Labels: map[string]string{
						ManagedByLabel: ManagedByValue,
					},
				},
				Provisioner: "example.test/provisioner",
			},
			&corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "other",
					Name:      "data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &storageClassName,
				},
			},
		)),
		observability.MustNew(cfg.Observability, nil),
	)

	items, err := service.ListStorageClasses(t.Context(), true)
	if err != nil {
		t.Fatalf("ListStorageClasses() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if !items[0].ManagedByStorageManager || items[0].InUsePVCCount != 1 || items[0].DeleteBlockedReason != "in_use" {
		t.Fatalf("item = %#v", items[0])
	}
}

func TestStorageClassYAMLValidation(t *testing.T) {
	t.Parallel()

	_, err := parseStorageClassYAML(`apiVersion: v1
kind: ConfigMap
metadata:
  name: nope
`)

	var apiErr *apienv.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("parseStorageClassYAML() error = %T %v, want apienv.Error", err, err)
	}
	if apiErr.Code != apienv.CodeStorageClassYAMLInvalid {
		t.Fatalf("code = %s, want %s", apiErr.Code, apienv.CodeStorageClassYAMLInvalid)
	}
}

func TestStorageClassServiceListReturnsAllStorageClasses(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	service := NewStorageClassService(
		kube.New(fake.NewSimpleClientset(
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "visible",
				},
				Provisioner: "example.test/provisioner",
			},
			&storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: "hidden"},
				Provisioner: "example.test/provisioner",
			},
		)),
		observability.MustNew(cfg.Observability, nil),
	)

	visible, err := service.ListStorageClasses(t.Context(), false)
	if err != nil {
		t.Fatalf("ListStorageClasses(false) error = %v", err)
	}
	if len(visible) != 2 {
		t.Fatalf("visible = %#v", visible)
	}
	all, err := service.ListStorageClasses(t.Context(), true)
	if err != nil {
		t.Fatalf("ListStorageClasses(true) error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all = %#v", all)
	}
}
