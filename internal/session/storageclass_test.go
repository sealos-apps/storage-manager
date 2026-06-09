package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStorageClassAccessPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		annotations map[string]string
		wantModes   []string
		wantStatus  string
	}{
		{
			name:       "hidden without annotations",
			wantModes:  []string{},
			wantStatus: storageClassAnnotationHidden,
		},
		{
			name: "ready",
			annotations: map[string]string{
				StorageClassVisibleAnnotation:     "true",
				StorageClassAccessModesAnnotation: "ReadWriteOnce, ReadWriteMany",
			},
			wantModes:  []string{domain.AccessModeReadWriteOnce, domain.AccessModeReadWriteMany},
			wantStatus: storageClassAnnotationReady,
		},
		{
			name: "invalid unsupported mode",
			annotations: map[string]string{
				StorageClassVisibleAnnotation:     "true",
				StorageClassAccessModesAnnotation: "ReadWriteOncePod",
			},
			wantModes:  []string{},
			wantStatus: storageClassAnnotationInvalid,
		},
		{
			name: "invalid empty modes",
			annotations: map[string]string{
				StorageClassVisibleAnnotation: "true",
			},
			wantModes:  []string{},
			wantStatus: storageClassAnnotationInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotModes, gotStatus := StorageClassAccessPolicy(tt.annotations)

			if strings.Join(gotModes, ",") != strings.Join(tt.wantModes, ",") {
				t.Fatalf("modes = %#v, want %#v", gotModes, tt.wantModes)
			}
			if gotStatus != tt.wantStatus {
				t.Fatalf("status = %q, want %q", gotStatus, tt.wantStatus)
			}
		})
	}
}

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
  annotations:
    storage-management.sealos.io/visible-in-create: "true"
    storage-management.sealos.io/access-modes: "ReadWriteOnce,ReadWriteMany"
provisioner: example.test/provisioner
`

	created, err := service.CreateStorageClass(t.Context(), body)
	if err != nil {
		t.Fatalf("CreateStorageClass() error = %v", err)
	}
	if !created.VisibleInCreate || strings.Join(created.AllowedAccessModes, ",") != "ReadWriteOnce,ReadWriteMany" {
		t.Fatalf("created = %#v", created)
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
	if !strings.Contains(describe.Describe, "Visible In Create: true") {
		t.Fatalf("describe = %s", describe.Describe)
	}
	updated, err := service.UpdateStorageClass(t.Context(), "fast", strings.Replace(body, "ReadWriteOnce,ReadWriteMany", "ReadOnlyMany", 1))
	if err != nil {
		t.Fatalf("UpdateStorageClass() error = %v", err)
	}
	if strings.Join(updated.AllowedAccessModes, ",") != "ReadOnlyMany" {
		t.Fatalf("updated = %#v", updated)
	}
	policyUpdated, err := service.UpdateStorageClassPolicy(t.Context(), "fast", StorageClassPolicyInput{
		AllowedAccessModes: []string{"ReadWriteMany", "ReadWriteOnce", "ReadWriteMany"},
		VisibleInCreate:    true,
	})
	if err != nil {
		t.Fatalf("UpdateStorageClassPolicy() error = %v", err)
	}
	if strings.Join(policyUpdated.AllowedAccessModes, ",") != "ReadWriteMany,ReadWriteOnce" {
		t.Fatalf("policyUpdated = %#v", policyUpdated)
	}
	current, err := clientset.StorageV1().StorageClasses().Get(t.Context(), "fast", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get storageclass: %v", err)
	}
	if current.Annotations[StorageClassVisibleAnnotation] != "true" {
		t.Fatalf("visible annotation = %q", current.Annotations[StorageClassVisibleAnnotation])
	}
	if current.Annotations[StorageClassAccessModesAnnotation] != "ReadWriteMany,ReadWriteOnce" {
		t.Fatalf("access modes annotation = %q", current.Annotations[StorageClassAccessModesAnnotation])
	}
	hidden, err := service.UpdateStorageClassPolicy(t.Context(), "fast", StorageClassPolicyInput{
		VisibleInCreate: false,
	})
	if err != nil {
		t.Fatalf("UpdateStorageClassPolicy(hidden) error = %v", err)
	}
	if hidden.VisibleInCreate || len(hidden.AllowedAccessModes) != 0 {
		t.Fatalf("hidden = %#v", hidden)
	}
	current, err = clientset.StorageV1().StorageClasses().Get(t.Context(), "fast", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get hidden storageclass: %v", err)
	}
	if _, ok := current.Annotations[StorageClassAccessModesAnnotation]; ok {
		t.Fatalf("access modes annotation still present: %#v", current.Annotations)
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

func TestStorageClassServiceListFiltersHidden(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	service := NewStorageClassService(
		kube.New(fake.NewSimpleClientset(
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "visible",
					Annotations: map[string]string{
						StorageClassVisibleAnnotation:     "true",
						StorageClassAccessModesAnnotation: "ReadWriteOnce",
					},
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
	if len(visible) != 1 || visible[0].Name != "visible" {
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
