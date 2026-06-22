package viewer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var errRuntimeUnavailable = errors.New("viewer runtime is not configured")

type kubernetesAuthorizer struct {
	management           kubernetes.Interface
	recorder             *observability.Recorder
	managementRESTConfig *rest.Config
}

func newKubernetesAuthorizer(
	management kubernetes.Interface,
	recorder *observability.Recorder,
	managementRESTConfig *rest.Config,
) kubernetesAuthorizer {
	return kubernetesAuthorizer{
		management:           management,
		recorder:             recorder,
		managementRESTConfig: managementRESTConfig,
	}
}

func (a kubernetesAuthorizer) CanListPVCs(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx,
		"kubernetes.authorize.list_pvcs",
		slog.String("namespace", namespace),
		slog.Bool("management_client", a.management != nil),
	)
	defer func() {
		finish(err)
	}()

	clientset, err := a.clientsetForPrincipal(principal)
	if err != nil {
		return err
	}
	if a.management != nil {
		if err := a.sameNamespace(ctx, clientset, namespace); err != nil {
			return err
		}
	}
	return a.observeKubernetes(ctx, "list", "persistentvolumeclaims", namespace, "", func(ctx context.Context) error {
		_, err := clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{Limit: 1})
		return err
	})
}

func (a kubernetesAuthorizer) CanGetPVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
	name string,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx,
		"kubernetes.authorize.get_pvc",
		slog.String("namespace", namespace),
		slog.String("pvc_name", name),
		slog.Bool("management_client", a.management != nil),
	)
	defer func() {
		finish(err)
	}()

	clientset, err := a.clientsetForPrincipal(principal)
	if err != nil {
		return err
	}
	var userPVCUID string
	err = a.observeKubernetes(ctx, "get", "persistentvolumeclaim", namespace, name, func(ctx context.Context) error {
		userPVC, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		userPVCUID = string(userPVC.UID)
		return nil
	})
	if a.management == nil {
		return err
	}
	if err != nil {
		return err
	}
	var managedPVCUID string
	err = a.observeKubernetes(ctx, "get", "persistentvolumeclaim", namespace, name, func(ctx context.Context) error {
		managedPVC, err := a.management.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		managedPVCUID = string(managedPVC.UID)
		return nil
	})
	if err != nil {
		return err
	}
	if userPVCUID != managedPVCUID {
		return errors.New("user kubeconfig and management kubeconfig resolved different PVCs")
	}
	return nil
}

func (a kubernetesAuthorizer) CanCreatePVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx,
		"kubernetes.authorize.create_pvc",
		slog.String("namespace", namespace),
		slog.Bool("management_client", a.management != nil),
	)
	defer func() {
		finish(err)
	}()

	clientset, err := a.clientsetForPrincipal(principal)
	if err != nil {
		return err
	}
	if a.management != nil {
		if err := a.sameNamespace(ctx, clientset, namespace); err != nil {
			return err
		}
	}
	return a.canUseResource(ctx, clientset, "create", "", "persistentvolumeclaims", namespace, "")
}

func (a kubernetesAuthorizer) CanDeletePVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
	name string,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx,
		"kubernetes.authorize.delete_pvc",
		slog.String("namespace", namespace),
		slog.String("pvc_name", name),
		slog.Bool("management_client", a.management != nil),
	)
	defer func() {
		finish(err)
	}()

	if err := a.CanGetPVC(ctx, principal, namespace, name); err != nil {
		return err
	}
	clientset, err := a.clientsetForPrincipal(principal)
	if err != nil {
		return err
	}
	return a.canUseResource(ctx, clientset, "delete", "", "persistentvolumeclaims", namespace, name)
}

func (a kubernetesAuthorizer) CanUpdatePVC(
	ctx context.Context,
	principal *authn.Principal,
	namespace string,
	name string,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx,
		"kubernetes.authorize.update_pvc",
		slog.String("namespace", namespace),
		slog.String("pvc_name", name),
		slog.Bool("management_client", a.management != nil),
	)
	defer func() {
		finish(err)
	}()

	if err := a.CanGetPVC(ctx, principal, namespace, name); err != nil {
		return err
	}
	clientset, err := a.clientsetForPrincipal(principal)
	if err != nil {
		return err
	}
	return a.canUseResource(ctx, clientset, "update", "", "persistentvolumeclaims", namespace, name)
}

func (a kubernetesAuthorizer) CanListStorageClasses(
	ctx context.Context,
	principal *authn.Principal,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx, "kubernetes.authorize.list_storageclasses")
	defer func() {
		finish(err)
	}()

	clientset, err := a.clientsetForPrincipal(principal)
	if err != nil {
		return err
	}
	return a.canUseResource(ctx, clientset, "list", "storage.k8s.io", "storageclasses", "", "")
}

func (a kubernetesAuthorizer) sameNamespace(
	ctx context.Context,
	userClient kubernetes.Interface,
	namespace string,
) error {
	var userNamespaceUID string
	err := a.observeKubernetes(ctx, "get", "namespace", namespace, "", func(ctx context.Context) error {
		userNamespace, err := userClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			return err
		}
		userNamespaceUID = string(userNamespace.UID)
		return nil
	})
	if err != nil {
		return err
	}
	var managedNamespaceUID string
	err = a.observeKubernetes(ctx, "get", "namespace", namespace, "", func(ctx context.Context) error {
		managedNamespace, err := a.management.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			return err
		}
		managedNamespaceUID = string(managedNamespace.UID)
		return nil
	})
	if err != nil {
		return err
	}
	if userNamespaceUID != managedNamespaceUID {
		return errors.New("user kubeconfig and management kubeconfig resolved different namespaces")
	}
	return nil
}

func (a kubernetesAuthorizer) clientsetForPrincipal(principal *authn.Principal) (kubernetes.Interface, error) {
	return kubernetesClientsetForConfig(clientConfigForPrincipal(a.managementRESTConfig, principal))
}

func (a kubernetesAuthorizer) observeKubernetes(
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
	ctx, finish := a.recorder.TraceOperation(ctx, "kubernetes."+operation, attrs...)
	defer func() {
		a.recorder.ObserveKubernetes(operation, resource, err, time.Since(start))
		finish(err)
	}()
	return call(ctx)
}

func (a kubernetesAuthorizer) canUseResource(
	ctx context.Context,
	clientset kubernetes.Interface,
	verb string,
	group string,
	resource string,
	namespace string,
	name string,
) error {
	return a.observeKubernetes(ctx, verb, resource, namespace, name, func(ctx context.Context) error {
		review, err := clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(
			ctx,
			&authorizationv1.SelfSubjectAccessReview{
				Spec: authorizationv1.SelfSubjectAccessReviewSpec{
					ResourceAttributes: &authorizationv1.ResourceAttributes{
						Group:     group,
						Verb:      verb,
						Resource:  resource,
						Namespace: namespace,
						Name:      name,
					},
				},
			},
			metav1.CreateOptions{},
		)
		if err != nil {
			return err
		}
		if !review.Status.Allowed {
			return errors.New("self subject access review denied")
		}
		return nil
	})
}
