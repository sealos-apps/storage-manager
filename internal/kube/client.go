package kube

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	GetPVC(ctx context.Context, namespace string, name string) (*corev1.PersistentVolumeClaim, error)
	ListPVCs(ctx context.Context, namespace string) ([]corev1.PersistentVolumeClaim, error)
	CreatePVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error)
	DeletePVC(ctx context.Context, namespace string, name string) error
	UpdatePVCStorageRequest(
		ctx context.Context,
		namespace string,
		name string,
		storage resource.Quantity,
	) (*corev1.PersistentVolumeClaim, error)
	GetStorageClass(ctx context.Context, name string) (*storagev1.StorageClass, error)
	ListStorageClasses(ctx context.Context) ([]storagev1.StorageClass, error)
	CreateStorageClass(ctx context.Context, storageClass *storagev1.StorageClass) (*storagev1.StorageClass, error)
	UpdateStorageClass(ctx context.Context, storageClass *storagev1.StorageClass) (*storagev1.StorageClass, error)
	DeleteStorageClass(ctx context.Context, name string) error
	ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error)
	ListViewerPods(ctx context.Context, namespace string, labels map[string]string) ([]corev1.Pod, error)
	GetPod(ctx context.Context, namespace string, name string) (*corev1.Pod, error)
	CreatePod(ctx context.Context, pod *corev1.Pod) (*corev1.Pod, error)
	PatchPodAnnotations(
		ctx context.Context,
		namespace string,
		name string,
		annotations map[string]string,
	) (*corev1.Pod, error)
	DeletePod(ctx context.Context, namespace string, name string) error
	CreateService(ctx context.Context, service *corev1.Service) (*corev1.Service, error)
	CreateIngress(ctx context.Context, ingress *networkingv1.Ingress) (*networkingv1.Ingress, error)
	CreateConfigMap(ctx context.Context, configMap *corev1.ConfigMap) (*corev1.ConfigMap, error)
}

type Client struct {
	clientset kubernetes.Interface
}

func New(clientset kubernetes.Interface) *Client {
	return &Client{clientset: clientset}
}

func (c *Client) GetPVC(ctx context.Context, namespace string, name string) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pvc %s/%s: %w", namespace, name, err)
	}
	return pvc, nil
}

func (c *Client) ListPVCs(ctx context.Context, namespace string) ([]corev1.PersistentVolumeClaim, error) {
	list, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pvcs in %s: %w", namespace, err)
	}
	return list.Items, nil
}

func (c *Client) CreatePVC(
	ctx context.Context,
	pvc *corev1.PersistentVolumeClaim,
) (*corev1.PersistentVolumeClaim, error) {
	created, err := c.clientset.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating pvc %s/%s: %w", pvc.Namespace, pvc.Name, err)
	}
	return created, nil
}

func (c *Client) DeletePVC(ctx context.Context, namespace string, name string) error {
	if err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting pvc %s/%s: %w", namespace, name, err)
	}
	return nil
}

func (c *Client) UpdatePVCStorageRequest(
	ctx context.Context,
	namespace string,
	name string,
	storage resource.Quantity,
) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := c.GetPVC(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if pvc.Spec.Resources.Requests == nil {
		pvc.Spec.Resources.Requests = corev1.ResourceList{}
	}
	pvc.Spec.Resources.Requests[corev1.ResourceStorage] = storage
	updated, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, pvc, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("updating pvc %s/%s storage request: %w", namespace, name, err)
	}
	return updated, nil
}

func (c *Client) GetStorageClass(ctx context.Context, name string) (*storagev1.StorageClass, error) {
	storageClass, err := c.clientset.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting storageclass %s: %w", name, err)
	}
	return storageClass, nil
}

func (c *Client) ListStorageClasses(ctx context.Context) ([]storagev1.StorageClass, error) {
	list, err := c.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing storageclasses: %w", err)
	}
	return list.Items, nil
}

func (c *Client) CreateStorageClass(
	ctx context.Context,
	storageClass *storagev1.StorageClass,
) (*storagev1.StorageClass, error) {
	created, err := c.clientset.StorageV1().StorageClasses().Create(ctx, storageClass, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating storageclass %s: %w", storageClass.Name, err)
	}
	return created, nil
}

func (c *Client) UpdateStorageClass(
	ctx context.Context,
	storageClass *storagev1.StorageClass,
) (*storagev1.StorageClass, error) {
	updated, err := c.clientset.StorageV1().StorageClasses().Update(ctx, storageClass, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("updating storageclass %s: %w", storageClass.Name, err)
	}
	return updated, nil
}

func (c *Client) DeleteStorageClass(ctx context.Context, name string) error {
	if err := c.clientset.StorageV1().StorageClasses().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting storageclass %s: %w", name, err)
	}
	return nil
}

func (c *Client) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	list, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods in %s: %w", namespace, err)
	}
	return list.Items, nil
}

func (c *Client) ListViewerPods(
	ctx context.Context,
	namespace string,
	labels map[string]string,
) ([]corev1.Pod, error) {
	selector := metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: labels})
	list, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("listing viewer pods in %s: %w", namespace, err)
	}
	return list.Items, nil
}

func (c *Client) GetPod(ctx context.Context, namespace string, name string) (*corev1.Pod, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod %s/%s: %w", namespace, name, err)
	}
	return pod, nil
}

func (c *Client) CreatePod(ctx context.Context, pod *corev1.Pod) (*corev1.Pod, error) {
	created, err := c.clientset.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	return created, nil
}

func (c *Client) PatchPodAnnotations(
	ctx context.Context,
	namespace string,
	name string,
	annotations map[string]string,
) (*corev1.Pod, error) {
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": annotations,
		},
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("marshaling pod annotation patch %s/%s: %w", namespace, name, err)
	}
	pod, err := c.clientset.CoreV1().Pods(namespace).Patch(
		ctx,
		name,
		types.MergePatchType,
		body,
		metav1.PatchOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("patching pod annotations %s/%s: %w", namespace, name, err)
	}
	return pod, nil
}

func (c *Client) DeletePod(ctx context.Context, namespace string, name string) error {
	if err := c.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting pod %s/%s: %w", namespace, name, err)
	}
	return nil
}

func (c *Client) CreateService(ctx context.Context, service *corev1.Service) (*corev1.Service, error) {
	created, err := c.clientset.CoreV1().Services(service.Namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating service %s/%s: %w", service.Namespace, service.Name, err)
	}
	return created, nil
}

func (c *Client) CreateIngress(
	ctx context.Context,
	ingress *networkingv1.Ingress,
) (*networkingv1.Ingress, error) {
	created, err := c.clientset.NetworkingV1().Ingresses(ingress.Namespace).Create(ctx, ingress, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating ingress %s/%s: %w", ingress.Namespace, ingress.Name, err)
	}
	return created, nil
}

func (c *Client) CreateConfigMap(
	ctx context.Context,
	configMap *corev1.ConfigMap,
) (*corev1.ConfigMap, error) {
	created, err := c.clientset.CoreV1().ConfigMaps(configMap.Namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating configmap %s/%s: %w", configMap.Namespace, configMap.Name, err)
	}
	return created, nil
}
