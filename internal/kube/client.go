package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	GetPVC(ctx context.Context, namespace string, name string) (*corev1.PersistentVolumeClaim, error)
	ListPVCs(ctx context.Context, namespace string) ([]corev1.PersistentVolumeClaim, error)
	ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error)
	ListViewerPods(ctx context.Context, namespace string, labels map[string]string) ([]corev1.Pod, error)
	GetPod(ctx context.Context, namespace string, name string) (*corev1.Pod, error)
	CreatePod(ctx context.Context, pod *corev1.Pod) (*corev1.Pod, error)
	DeletePod(ctx context.Context, namespace string, name string) error
	CreateService(ctx context.Context, service *corev1.Service) (*corev1.Service, error)
	DeleteService(ctx context.Context, namespace string, name string) error
	CreateIngress(ctx context.Context, ingress *networkingv1.Ingress) (*networkingv1.Ingress, error)
	DeleteIngress(ctx context.Context, namespace string, name string) error
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

func (c *Client) DeleteService(ctx context.Context, namespace string, name string) error {
	if err := c.clientset.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting service %s/%s: %w", namespace, name, err)
	}
	return nil
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

func (c *Client) DeleteIngress(ctx context.Context, namespace string, name string) error {
	if err := c.clientset.NetworkingV1().Ingresses(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting ingress %s/%s: %w", namespace, name, err)
	}
	return nil
}
