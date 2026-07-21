package kube

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestClientListPVCReferencesFromWorkload(t *testing.T) {
	t.Parallel()

	client := New(kubefake.NewSimpleClientset(
		testPVCObject("default", "data"),
		testPVCObject("default", "shared"),
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "demo",
				UID:       types.UID("deployment-uid"),
				Labels: map[string]string{
					PVCReferenceSourceLabel:  "true",
					PVCReferenceProductLabel: "applaunchpad",
					PVCReferenceKindLabel:    "App",
				},
				Annotations: map[string]string{
					PVCReferenceNameAnnotation: "Demo App",
					PVCReferencesAnnotation:    `[{"name":"data","relation":"mounted","mountPath":"/data"},{"name":"missing"}]`,
				},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:         "app",
							VolumeMounts: []corev1.VolumeMount{{Name: "shared", MountPath: "/shared"}},
						}},
						Volumes: []corev1.Volume{{
							Name: "shared",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "shared"},
							},
						}},
					},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "unmarked"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{{
							Name: "ignored",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"},
							},
						}},
					},
				},
			},
		},
	))

	references, err := client.ListPVCReferences(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListPVCReferences() error = %v", err)
	}
	if len(references) != 2 {
		t.Fatalf("references = %#v", references)
	}
	byPVC := pvcReferenceBindingsByPVC(references)
	if got := byPVC["data"][0].Reference; got.SourceProduct != "applaunchpad" ||
		got.SourceKind != "App" ||
		got.SourceName != "Demo App" ||
		got.MountPath != "/data" ||
		got.Evidence != PVCReferenceEvidenceAnnotation {
		t.Fatalf("data reference = %#v", got)
	}
	if got := byPVC["shared"][0].Reference; got.MountPath != "/shared" ||
		got.Evidence != PVCReferenceEvidenceWorkloadVolume {
		t.Fatalf("shared reference = %#v", got)
	}
}

func TestClientListPVCReferencesFromStatefulSetClaimTemplates(t *testing.T) {
	t.Parallel()

	client := New(kubefake.NewSimpleClientset(
		testPVCObject("default", "data-demo-0"),
		testPVCObject("default", "data-demo-1"),
		testPVCObject("default", "data-other-0"),
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "demo",
				Labels: map[string]string{
					PVCReferenceSourceLabel:  "true",
					PVCReferenceProductLabel: "applaunchpad",
					PVCReferenceKindLabel:    "App",
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:         "app",
							VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/data"}},
						}},
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
				}},
			},
		},
	))

	references, err := client.ListPVCReferences(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListPVCReferences() error = %v", err)
	}
	if len(references) != 2 {
		t.Fatalf("references = %#v", references)
	}
	for _, reference := range references {
		if reference.PVCName == "data-other-0" {
			t.Fatalf("unexpected other statefulset reference: %#v", reference)
		}
		if reference.Reference.Relation != PVCReferenceRelationOwned ||
			reference.Reference.Evidence != PVCReferenceEvidenceStatefulSetClaimName ||
			reference.Reference.MountPath != "/data" {
			t.Fatalf("reference = %#v", reference)
		}
	}
}

func TestClientListPVCReferencesFromDynamicCustomResource(t *testing.T) {
	t.Parallel()

	devbox := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "devbox.sealos.io/v1alpha2",
			"kind":       "Devbox",
		},
	}
	devbox.SetNamespace("default")
	devbox.SetName("code-server")
	devbox.SetUID(types.UID("devbox-uid"))
	devbox.SetLabels(map[string]string{
		PVCReferenceSourceLabel:  "true",
		PVCReferenceProductLabel: "devbox",
		PVCReferenceKindLabel:    "DevBox",
	})
	devbox.SetAnnotations(map[string]string{
		PVCReferenceNameAnnotation: "code-server",
		PVCReferencesAnnotation:    `[{"name":"workspace","mountPath":"/workspace"}]`,
	})
	dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			pvcReferenceCustomGVRs[0]: "DevboxList",
			pvcReferenceCustomGVRs[1]: "DevboxList",
			pvcReferenceCustomGVRs[2]: "AppList",
		},
	)
	dynamicClient.PrependReactor("list", "devboxes", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetResource().Version != "v1alpha2" {
			return true, &unstructured.UnstructuredList{}, nil
		}
		return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*devbox}}, nil
	})
	client := NewWithDynamic(kubefake.NewSimpleClientset(testPVCObject("default", "workspace")), dynamicClient)

	references, err := client.ListPVCReferences(t.Context(), "default")
	if err != nil {
		t.Fatalf("ListPVCReferences() error = %v", err)
	}
	if len(references) != 1 {
		t.Fatalf("references = %#v", references)
	}
	got := references[0].Reference
	if got.SourceProduct != "devbox" ||
		got.SourceKind != "DevBox" ||
		got.SourceUID != "devbox-uid" ||
		got.MountPath != "/workspace" ||
		got.Relation != PVCReferenceRelationMounted {
		t.Fatalf("reference = %#v", got)
	}
}

func TestClientListPVCReferencesIgnoresCrossNamespaceDeclaration(t *testing.T) {
	t.Parallel()

	client := New(kubefake.NewSimpleClientset(
		testPVCObject("other", "data"),
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "demo",
				Labels: map[string]string{
					PVCReferenceSourceLabel: "true",
				},
				Annotations: map[string]string{
					PVCReferencesAnnotation: `[{"name":"data","namespace":"other"}]`,
				},
			},
		},
	))

	references, err := client.ListAllPVCReferences(t.Context())
	if err != nil {
		t.Fatalf("ListAllPVCReferences() error = %v", err)
	}
	if len(references) != 0 {
		t.Fatalf("references = %#v, want none", references)
	}
}

func TestClientListPVCReferencesRejectsMalformedDeclaration(t *testing.T) {
	t.Parallel()

	client := New(kubefake.NewSimpleClientset(
		testPVCObject("default", "data"),
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "demo",
				Labels:    map[string]string{PVCReferenceSourceLabel: "true"},
				Annotations: map[string]string{
					PVCReferencesAnnotation: `not-json`,
				},
			},
		},
	))

	if _, err := client.ListPVCReferences(t.Context(), "default"); err == nil {
		t.Fatal("ListPVCReferences() error = nil")
	}
}

func testPVCObject(namespace string, name string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
}

func pvcReferenceBindingsByPVC(references []PVCReferenceBinding) map[string][]PVCReferenceBinding {
	byPVC := map[string][]PVCReferenceBinding{}
	for _, reference := range references {
		byPVC[reference.PVCName] = append(byPVC[reference.PVCName], reference)
	}
	return byPVC
}
