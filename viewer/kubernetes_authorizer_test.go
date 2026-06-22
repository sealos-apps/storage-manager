package viewer

import (
	"net/url"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

func TestKubernetesAuthorizerUsesManagementHostForStorageClassSSAR(t *testing.T) {
	clientsetFactoryMu.Lock()
	defer clientsetFactoryMu.Unlock()

	principal, err := authn.PrincipalFromAuthorization(url.QueryEscape(testKubeconfig))
	if err != nil {
		t.Fatalf("PrincipalFromAuthorization() error = %v", err)
	}
	principal.ClientConfig.Host = "https://user.example.invalid"
	authorizer := newKubernetesAuthorizer(
		fake.NewSimpleClientset(),
		observability.MustNew(testObservability(), nil),
		&rest.Config{Host: "https://management.example.invalid", BearerToken: "management-token"},
	)
	clientset := fake.NewSimpleClientset()
	clientset.Fake.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authorizationv1.SelfSubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true},
		}, nil
	})
	var got *rest.Config
	newClientset := kubernetesClientsetForConfig
	kubernetesClientsetForConfig = func(c *rest.Config) (kubernetes.Interface, error) {
		got = c
		return clientset, nil
	}
	defer func() {
		kubernetesClientsetForConfig = newClientset
	}()

	if err := authorizer.CanListStorageClasses(t.Context(), principal); err != nil {
		t.Fatalf("CanListStorageClasses() error = %v", err)
	}
	if got.Host != "https://management.example.invalid" {
		t.Fatalf("Host = %q, want management host", got.Host)
	}
	if got.BearerToken != "test-token" {
		t.Fatalf("BearerToken = %q, want user token", got.BearerToken)
	}
}

func TestKubernetesAuthorizerUsesManagementHostForPVCChecks(t *testing.T) {
	clientsetFactoryMu.Lock()
	defer clientsetFactoryMu.Unlock()

	principal, err := authn.PrincipalFromAuthorization(url.QueryEscape(testKubeconfig))
	if err != nil {
		t.Fatalf("PrincipalFromAuthorization() error = %v", err)
	}
	principal.ClientConfig.Host = "https://user.example.invalid"
	userClient := fake.NewSimpleClientset(namespaceWithUID("ns", "same-uid"))
	authorizer := newKubernetesAuthorizer(
		fake.NewSimpleClientset(namespaceWithUID("ns", "same-uid")),
		observability.MustNew(testObservability(), nil),
		&rest.Config{Host: "https://management.example.invalid", BearerToken: "management-token"},
	)
	var got *rest.Config
	newClientset := kubernetesClientsetForConfig
	kubernetesClientsetForConfig = func(c *rest.Config) (kubernetes.Interface, error) {
		got = c
		return userClient, nil
	}
	defer func() {
		kubernetesClientsetForConfig = newClientset
	}()

	if err := authorizer.CanListPVCs(t.Context(), principal, "ns"); err != nil {
		t.Fatalf("CanListPVCs() error = %v", err)
	}
	if got.Host != "https://management.example.invalid" {
		t.Fatalf("Host = %q, want management host", got.Host)
	}
	if got.BearerToken != "test-token" {
		t.Fatalf("BearerToken = %q, want user token", got.BearerToken)
	}
}

func TestKubernetesAuthorizerRequiresSameNamespaceUID(t *testing.T) {
	clientsetFactoryMu.Lock()
	defer clientsetFactoryMu.Unlock()

	principal, err := authn.PrincipalFromAuthorization(url.QueryEscape(testKubeconfig))
	if err != nil {
		t.Fatalf("PrincipalFromAuthorization() error = %v", err)
	}
	userClient := fake.NewSimpleClientset(namespaceWithUID("ns", "user-uid"))
	authorizer := newKubernetesAuthorizer(
		fake.NewSimpleClientset(namespaceWithUID("ns", "managed-uid")),
		observability.MustNew(testObservability(), nil),
		nil,
	)
	newClientset := kubernetesClientsetForConfig
	kubernetesClientsetForConfig = func(_ *rest.Config) (kubernetes.Interface, error) {
		return userClient, nil
	}
	defer func() {
		kubernetesClientsetForConfig = newClientset
	}()

	if err := authorizer.CanListPVCs(t.Context(), principal, "ns"); err == nil {
		t.Fatal("CanListPVCs() allowed namespace UID mismatch")
	}
}

func TestKubernetesAuthorizerRequiresSamePVCUID(t *testing.T) {
	clientsetFactoryMu.Lock()
	defer clientsetFactoryMu.Unlock()

	principal, err := authn.PrincipalFromAuthorization(url.QueryEscape(testKubeconfig))
	if err != nil {
		t.Fatalf("PrincipalFromAuthorization() error = %v", err)
	}
	userClient := fake.NewSimpleClientset(pvcWithUID("ns", "data", "user-uid"))
	authorizer := newKubernetesAuthorizer(
		fake.NewSimpleClientset(pvcWithUID("ns", "data", "managed-uid")),
		observability.MustNew(testObservability(), nil),
		nil,
	)
	newClientset := kubernetesClientsetForConfig
	kubernetesClientsetForConfig = func(_ *rest.Config) (kubernetes.Interface, error) {
		return userClient, nil
	}
	defer func() {
		kubernetesClientsetForConfig = newClientset
	}()

	if err := authorizer.CanGetPVC(t.Context(), principal, "ns", "data"); err == nil {
		t.Fatal("CanGetPVC() allowed PVC UID mismatch")
	}
}
