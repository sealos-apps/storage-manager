package kube

import (
	"context"
	"log/slog"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

type observedClient struct {
	next     Interface
	recorder *observability.Recorder
}

func WithObservability(next Interface, recorder *observability.Recorder) Interface {
	return observedClient{
		next:     next,
		recorder: recorder,
	}
}

func (c observedClient) GetPVC(
	ctx context.Context,
	namespace string,
	name string,
) (*corev1.PersistentVolumeClaim, error) {
	var pvc *corev1.PersistentVolumeClaim
	err := c.observe(ctx, "get", "persistentvolumeclaim", namespace, name, func(ctx context.Context) error {
		var err error
		pvc, err = c.next.GetPVC(ctx, namespace, name)
		return err
	})
	return pvc, err
}

func (c observedClient) ListPVCs(ctx context.Context, namespace string) ([]corev1.PersistentVolumeClaim, error) {
	var pvcs []corev1.PersistentVolumeClaim
	err := c.observe(ctx, "list", "persistentvolumeclaims", namespace, "", func(ctx context.Context) error {
		var err error
		pvcs, err = c.next.ListPVCs(ctx, namespace)
		return err
	})
	return pvcs, err
}

func (c observedClient) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	var pods []corev1.Pod
	err := c.observe(ctx, "list", "pods", namespace, "", func(ctx context.Context) error {
		var err error
		pods, err = c.next.ListPods(ctx, namespace)
		return err
	})
	return pods, err
}

func (c observedClient) ListViewerPods(
	ctx context.Context,
	namespace string,
	labels map[string]string,
) ([]corev1.Pod, error) {
	var pods []corev1.Pod
	err := c.observe(ctx, "list", "viewer_pods", namespace, "", func(ctx context.Context) error {
		var err error
		pods, err = c.next.ListViewerPods(ctx, namespace, labels)
		return err
	})
	return pods, err
}

func (c observedClient) GetPod(ctx context.Context, namespace string, name string) (*corev1.Pod, error) {
	var pod *corev1.Pod
	err := c.observe(ctx, "get", "pod", namespace, name, func(ctx context.Context) error {
		var err error
		pod, err = c.next.GetPod(ctx, namespace, name)
		return err
	})
	return pod, err
}

func (c observedClient) CreatePod(ctx context.Context, pod *corev1.Pod) (*corev1.Pod, error) {
	var created *corev1.Pod
	err := c.observe(ctx, "create", "pod", pod.Namespace, pod.Name, func(ctx context.Context) error {
		var err error
		created, err = c.next.CreatePod(ctx, pod)
		return err
	})
	return created, err
}

func (c observedClient) DeletePod(ctx context.Context, namespace string, name string) error {
	return c.observe(ctx, "delete", "pod", namespace, name, func(ctx context.Context) error {
		return c.next.DeletePod(ctx, namespace, name)
	})
}

func (c observedClient) CreateService(ctx context.Context, service *corev1.Service) (*corev1.Service, error) {
	var created *corev1.Service
	err := c.observe(ctx, "create", "service", service.Namespace, service.Name, func(ctx context.Context) error {
		var err error
		created, err = c.next.CreateService(ctx, service)
		return err
	})
	return created, err
}

func (c observedClient) CreateIngress(
	ctx context.Context,
	ingress *networkingv1.Ingress,
) (*networkingv1.Ingress, error) {
	var created *networkingv1.Ingress
	err := c.observe(ctx, "create", "ingress", ingress.Namespace, ingress.Name, func(ctx context.Context) error {
		var err error
		created, err = c.next.CreateIngress(ctx, ingress)
		return err
	})
	return created, err
}

func (c observedClient) CreateConfigMap(ctx context.Context, configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	var created *corev1.ConfigMap
	err := c.observe(ctx, "create", "configmap", configMap.Namespace, configMap.Name, func(ctx context.Context) error {
		var err error
		created, err = c.next.CreateConfigMap(ctx, configMap)
		return err
	})
	return created, err
}

func (c observedClient) observe(
	ctx context.Context,
	operation string,
	resource string,
	namespace string,
	name string,
	call func(context.Context) error,
) (err error) {
	start := time.Now()
	attrs := []slog.Attr{
		slog.String("operation", operation),
		slog.String("resource", resource),
		slog.String("namespace", namespace),
	}
	if name != "" {
		attrs = append(attrs, slog.String("name", name))
	}
	ctx, finish := c.recorder.TraceOperation(ctx, "kubernetes."+operation, attrs...)
	defer func() {
		c.recorder.ObserveKubernetes(operation, resource, err, time.Since(start))
		finish(err)
	}()
	return call(ctx)
}
