package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	PVCReferenceSourceLabel     = "storage.sealos.io/pvc-reference-source"
	PVCReferenceSourceTypeLabel = "storage.sealos.io/ref-source"
	PVCReferenceNameAnnotation  = "storage.sealos.io/ref-name"
	PVCReferencesAnnotation     = "storage.sealos.io/pvc-references"

	PVCReferenceRelationMounted = "mounted"
	PVCReferenceRelationOwned   = "owned"

	PVCReferenceEvidenceAnnotation           = "metadata-annotation"
	PVCReferenceEvidenceWorkloadVolume       = "workload-volume"
	PVCReferenceEvidenceStatefulSetClaimName = "statefulset-volume-claim-template"
)

var pvcReferenceCustomGVRs = []schema.GroupVersionResource{
	{Group: "devbox.sealos.io", Version: "v1alpha2", Resource: "devboxes"},
	{Group: "devbox.sealos.io", Version: "v1alpha1", Resource: "devboxes"},
	{Group: "app.sealos.io", Version: "v1", Resource: "apps"},
}

type PVCReferenceBinding struct {
	PVCNamespace string
	PVCName      string
	Reference    domain.PVCReference
}

type pvcReferenceSource struct {
	SourceType  string
	Namespace   string
	Name        string
	DisplayName string
	UID         string
	Labels      map[string]string
	Annotations map[string]string
}

type declaredPVCReference struct {
	Name           string `json:"name"`
	ClaimName      string `json:"claimName"`
	ClaimNameSnake string `json:"claim_name"`
	Namespace      string `json:"namespace"`
	Relation       string `json:"relation"`
	MountPath      string `json:"mountPath"`
	MountPathSnake string `json:"mount_path"`
}

func (c *Client) ListPVCReferences(ctx context.Context, namespace string) ([]PVCReferenceBinding, error) {
	pvcs, err := c.listReferencePVCs(ctx, namespace)
	if err != nil {
		return nil, err
	}
	return c.ListPVCReferencesForPVCs(ctx, namespace, pvcs)
}

func (c *Client) ListAllPVCReferences(ctx context.Context) ([]PVCReferenceBinding, error) {
	pvcs, err := c.listReferencePVCs(ctx, "")
	if err != nil {
		return nil, err
	}
	return c.ListAllPVCReferencesForPVCs(ctx, pvcs)
}

func (c *Client) ListPVCReferencesForPVCs(
	ctx context.Context,
	namespace string,
	pvcs []corev1.PersistentVolumeClaim,
) ([]PVCReferenceBinding, error) {
	return c.listPVCReferences(ctx, namespace, pvcNamesByNamespace(pvcs))
}

func (c *Client) ListAllPVCReferencesForPVCs(
	ctx context.Context,
	pvcs []corev1.PersistentVolumeClaim,
) ([]PVCReferenceBinding, error) {
	return c.listPVCReferences(ctx, "", pvcNamesByNamespace(pvcs))
}

func (c *Client) listPVCReferences(
	ctx context.Context,
	namespace string,
	pvcNamesByNamespace map[string]map[string]struct{},
) ([]PVCReferenceBinding, error) {
	var references []PVCReferenceBinding
	workloadRefs, err := c.listWorkloadPVCReferences(ctx, namespace, pvcNamesByNamespace)
	if err != nil {
		return nil, err
	}
	references = append(references, workloadRefs...)

	customRefs, err := c.listCustomPVCReferences(ctx, namespace)
	if err != nil {
		return nil, err
	}
	references = append(references, customRefs...)

	return dedupePVCReferenceBindings(filterPVCReferenceBindings(references, pvcNamesByNamespace)), nil
}

func (c *Client) listReferencePVCs(
	ctx context.Context,
	namespace string,
) ([]corev1.PersistentVolumeClaim, error) {
	list, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if namespace == "" {
			return nil, fmt.Errorf("listing all pvcs for reference detection: %w", err)
		}
		return nil, fmt.Errorf("listing pvcs in %s for reference detection: %w", namespace, err)
	}
	return list.Items, nil
}

func pvcNamesByNamespace(pvcs []corev1.PersistentVolumeClaim) map[string]map[string]struct{} {
	byNamespace := map[string]map[string]struct{}{}
	for _, pvc := range pvcs {
		if pvc.Namespace == "" || pvc.Name == "" {
			continue
		}
		if byNamespace[pvc.Namespace] == nil {
			byNamespace[pvc.Namespace] = map[string]struct{}{}
		}
		byNamespace[pvc.Namespace][pvc.Name] = struct{}{}
	}
	return byNamespace
}

func (c *Client) listWorkloadPVCReferences(
	ctx context.Context,
	namespace string,
	pvcNamesByNamespace map[string]map[string]struct{},
) ([]PVCReferenceBinding, error) {
	options := pvcReferenceSourceListOptions()
	var references []PVCReferenceBinding

	deployments, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, options)
	if err != nil {
		return nil, referenceListError("deployments", namespace, err)
	}
	for _, deployment := range deployments.Items {
		source := pvcReferenceSourceFromObject(&deployment.ObjectMeta, deployment.Kind, "Deployment")
		refs, err := pvcReferencesFromSource(source)
		if err != nil {
			return nil, err
		}
		refs = append(refs, pvcReferencesFromPodSpec(source, deployment.Spec.Template.Spec)...)
		references = append(references, refs...)
	}

	statefulSets, err := c.clientset.AppsV1().StatefulSets(namespace).List(ctx, options)
	if err != nil {
		return nil, referenceListError("statefulsets", namespace, err)
	}
	for _, statefulSet := range statefulSets.Items {
		source := pvcReferenceSourceFromObject(&statefulSet.ObjectMeta, statefulSet.Kind, "StatefulSet")
		refs, err := pvcReferencesFromSource(source)
		if err != nil {
			return nil, err
		}
		refs = append(refs, pvcReferencesFromPodSpec(source, statefulSet.Spec.Template.Spec)...)
		refs = append(refs, pvcReferencesFromStatefulSetClaims(source, statefulSet, pvcNamesByNamespace)...)
		references = append(references, refs...)
	}

	return references, nil
}

func (c *Client) listCustomPVCReferences(
	ctx context.Context,
	namespace string,
) ([]PVCReferenceBinding, error) {
	if c.dynamic == nil {
		return nil, nil
	}
	options := pvcReferenceSourceListOptions()
	var references []PVCReferenceBinding
	for _, gvr := range pvcReferenceCustomGVRs {
		list, err := c.dynamic.Resource(gvr).Namespace(namespace).List(ctx, options)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, referenceListError(gvr.String(), namespace, err)
		}
		for _, item := range list.Items {
			source := pvcReferenceSourceFromUnstructured(item)
			refs, err := pvcReferencesFromSource(source)
			if err != nil {
				return nil, err
			}
			references = append(references, refs...)
		}
	}
	return references, nil
}

func pvcReferenceSourceListOptions() metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: labels.Set{PVCReferenceSourceLabel: "true"}.String(),
	}
}

func pvcReferenceSourceFromObject(
	meta *metav1.ObjectMeta,
	kind string,
	fallbackKind string,
) pvcReferenceSource {
	if meta == nil {
		return pvcReferenceSource{SourceType: fallbackKind}
	}
	if kind == "" {
		kind = fallbackKind
	}
	source := pvcReferenceSource{
		SourceType: firstNonEmpty(
			strings.TrimSpace(meta.Labels[PVCReferenceSourceTypeLabel]),
			kind,
		),
		Namespace:   meta.Namespace,
		Name:        meta.Name,
		DisplayName: strings.TrimSpace(meta.Annotations[PVCReferenceNameAnnotation]),
		UID:         string(meta.UID),
		Labels:      meta.Labels,
		Annotations: meta.Annotations,
	}
	if source.DisplayName == "" {
		source.DisplayName = source.Name
	}
	return source
}

func pvcReferenceSourceFromUnstructured(item unstructured.Unstructured) pvcReferenceSource {
	meta := metav1.ObjectMeta{
		Namespace:   item.GetNamespace(),
		Name:        item.GetName(),
		UID:         item.GetUID(),
		Labels:      item.GetLabels(),
		Annotations: item.GetAnnotations(),
	}
	return pvcReferenceSourceFromObject(&meta, item.GetKind(), item.GetKind())
}

func pvcReferencesFromSource(source pvcReferenceSource) ([]PVCReferenceBinding, error) {
	body := strings.TrimSpace(source.Annotations[PVCReferencesAnnotation])
	if body == "" {
		return nil, nil
	}
	var declared []declaredPVCReference
	if err := json.Unmarshal([]byte(body), &declared); err != nil {
		return nil, fmt.Errorf("parsing pvc references on %s %s/%s: %w",
			source.SourceType,
			source.Namespace,
			source.Name,
			err,
		)
	}
	references := make([]PVCReferenceBinding, 0, len(declared))
	for index, item := range declared {
		name := firstNonEmpty(
			strings.TrimSpace(item.Name),
			strings.TrimSpace(item.ClaimName),
			strings.TrimSpace(item.ClaimNameSnake),
		)
		if name == "" {
			return nil, fmt.Errorf("pvc reference %d on %s/%s is missing name", index, source.Namespace, source.Name)
		}
		namespace := strings.TrimSpace(item.Namespace)
		if namespace == "" {
			namespace = source.Namespace
		}
		if namespace != source.Namespace {
			continue
		}
		relation := strings.TrimSpace(item.Relation)
		if relation == "" {
			relation = PVCReferenceRelationMounted
		}
		mountPath := firstNonEmpty(strings.TrimSpace(item.MountPath), strings.TrimSpace(item.MountPathSnake))
		references = append(references, PVCReferenceBinding{
			PVCNamespace: namespace,
			PVCName:      name,
			Reference:    domainReference(source, relation, mountPath, PVCReferenceEvidenceAnnotation),
		})
	}
	return references, nil
}

func pvcReferencesFromPodSpec(source pvcReferenceSource, podSpec corev1.PodSpec) []PVCReferenceBinding {
	mountPaths := mountPathsByVolumeName(podSpec)
	references := []PVCReferenceBinding{}
	for _, volume := range podSpec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}
		name := strings.TrimSpace(volume.PersistentVolumeClaim.ClaimName)
		if name == "" {
			continue
		}
		references = append(references, PVCReferenceBinding{
			PVCNamespace: source.Namespace,
			PVCName:      name,
			Reference: domainReference(
				source,
				PVCReferenceRelationMounted,
				mountPaths[volume.Name],
				PVCReferenceEvidenceWorkloadVolume,
			),
		})
	}
	return references
}

func pvcReferencesFromStatefulSetClaims(
	source pvcReferenceSource,
	statefulSet appsv1.StatefulSet,
	pvcNamesByNamespace map[string]map[string]struct{},
) []PVCReferenceBinding {
	namespacePVCs := pvcNamesByNamespace[statefulSet.Namespace]
	if len(namespacePVCs) == 0 || len(statefulSet.Spec.VolumeClaimTemplates) == 0 {
		return nil
	}
	mountPaths := mountPathsByVolumeName(statefulSet.Spec.Template.Spec)
	references := []PVCReferenceBinding{}
	for _, template := range statefulSet.Spec.VolumeClaimTemplates {
		templateName := strings.TrimSpace(template.Name)
		if templateName == "" {
			continue
		}
		for pvcName := range namespacePVCs {
			if !statefulSetClaimNameMatches(templateName, statefulSet.Name, pvcName) {
				continue
			}
			references = append(references, PVCReferenceBinding{
				PVCNamespace: statefulSet.Namespace,
				PVCName:      pvcName,
				Reference: domainReference(
					source,
					PVCReferenceRelationOwned,
					mountPaths[templateName],
					PVCReferenceEvidenceStatefulSetClaimName,
				),
			})
		}
	}
	return references
}

func mountPathsByVolumeName(podSpec corev1.PodSpec) map[string]string {
	paths := map[string]string{}
	collect := func(mounts []corev1.VolumeMount) {
		for _, mount := range mounts {
			if mount.Name == "" || mount.MountPath == "" {
				continue
			}
			if _, exists := paths[mount.Name]; !exists {
				paths[mount.Name] = mount.MountPath
			}
		}
	}
	for _, container := range podSpec.InitContainers {
		collect(container.VolumeMounts)
	}
	for _, container := range podSpec.Containers {
		collect(container.VolumeMounts)
	}
	for _, container := range podSpec.EphemeralContainers {
		collect(container.VolumeMounts)
	}
	return paths
}

func statefulSetClaimNameMatches(templateName string, statefulSetName string, pvcName string) bool {
	prefix := templateName + "-" + statefulSetName + "-"
	if !strings.HasPrefix(pvcName, prefix) {
		return false
	}
	ordinal := strings.TrimPrefix(pvcName, prefix)
	if ordinal == "" {
		return false
	}
	_, err := strconv.Atoi(ordinal)
	return err == nil
}

func domainReference(
	source pvcReferenceSource,
	relation string,
	mountPath string,
	evidence string,
) domain.PVCReference {
	return domain.PVCReference{
		SourceType:      source.SourceType,
		SourceNamespace: source.Namespace,
		SourceName:      firstNonEmpty(source.DisplayName, source.Name),
		SourceUID:       source.UID,
		Relation:        relation,
		MountPath:       mountPath,
		Evidence:        evidence,
	}
}

func dedupePVCReferenceBindings(references []PVCReferenceBinding) []PVCReferenceBinding {
	sort.Slice(references, func(i int, j int) bool {
		return pvcReferenceBindingSortKey(references[i]) < pvcReferenceBindingSortKey(references[j])
	})
	deduped := make([]PVCReferenceBinding, 0, len(references))
	seen := map[string]struct{}{}
	for _, ref := range references {
		key := pvcReferenceBindingDedupeKey(ref)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, ref)
	}
	return deduped
}

func filterPVCReferenceBindings(
	references []PVCReferenceBinding,
	pvcNamesByNamespace map[string]map[string]struct{},
) []PVCReferenceBinding {
	if len(pvcNamesByNamespace) == 0 {
		return nil
	}
	filtered := make([]PVCReferenceBinding, 0, len(references))
	for _, ref := range references {
		if _, ok := pvcNamesByNamespace[ref.PVCNamespace][ref.PVCName]; !ok {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

func pvcReferenceBindingDedupeKey(ref PVCReferenceBinding) string {
	reference := ref.Reference
	return strings.Join([]string{
		ref.PVCNamespace,
		ref.PVCName,
		reference.SourceType,
		reference.SourceNamespace,
		reference.SourceName,
		reference.SourceUID,
		reference.Relation,
		reference.MountPath,
	}, "\x00")
}

func pvcReferenceBindingSortKey(ref PVCReferenceBinding) string {
	reference := ref.Reference
	return strings.Join([]string{
		ref.PVCNamespace,
		ref.PVCName,
		reference.SourceType,
		reference.SourceNamespace,
		reference.SourceName,
		reference.Relation,
		reference.MountPath,
		reference.Evidence,
	}, "\x00")
}

func referenceListError(resource string, namespace string, err error) error {
	if namespace == "" {
		return fmt.Errorf("listing %s for pvc reference detection: %w", resource, err)
	}
	return fmt.Errorf("listing %s in %s for pvc reference detection: %w", resource, namespace, err)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
