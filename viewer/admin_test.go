package viewer

import (
	"net/url"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

func TestKubernetesAdminAuthorizerUsesManagementHostForSelfSubjectReview(t *testing.T) {
	clientsetFactoryMu.Lock()
	defer clientsetFactoryMu.Unlock()

	principal, err := authn.PrincipalFromAuthorization(url.QueryEscape(testKubeconfig))
	if err != nil {
		t.Fatalf("PrincipalFromAuthorization() error = %v", err)
	}
	principal.ClientConfig.Host = "https://user.example.invalid"
	authorizer := newKubernetesAdminAuthorizer(
		config.AdminConfig{AllowedUserIDs: []string{"admin"}},
		observability.MustNew(testObservability(), nil),
		&rest.Config{Host: "https://management.example.invalid", BearerToken: "management-token"},
	)
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("create", "selfsubjectreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authenticationv1.SelfSubjectReview{
			Status: authenticationv1.SelfSubjectReviewStatus{
				UserInfo: authenticationv1.UserInfo{
					Username: sealosAdminUsername("admin"),
				},
			},
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

	if _, err := authorizer.CanAdmin(t.Context(), principal); err != nil {
		t.Fatalf("CanAdmin() error = %v", err)
	}
	if got.Host != "https://management.example.invalid" {
		t.Fatalf("Host = %q, want management host", got.Host)
	}
	if got.BearerToken != "test-token" {
		t.Fatalf("BearerToken = %q, want user token", got.BearerToken)
	}
}

func TestSealosUserNamespaceUsesAdminUserID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		user string
		want string
	}{
		{name: "admin", user: "admin", want: "ns-admin"},
		{name: "trim spaces", user: " admin ", want: "ns-admin"},
		{name: "generated user", user: "b4hw543c", want: "ns-b4hw543c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := sealosUserNamespace(tt.user); got != tt.want {
				t.Fatalf("sealosUserNamespace(%q) = %q, want %q", tt.user, got, tt.want)
			}
		})
	}
}

func TestIsAdminInOwnNamespaceRequiresExactAllowedNamespace(t *testing.T) {
	t.Parallel()

	result := AdminAuthorizationResult{
		Allowed:          true,
		AllowedNamespace: "ns-admin",
	}
	tests := []struct {
		name      string
		principal *authn.Principal
		result    AdminAuthorizationResult
		want      bool
	}{
		{
			name:      "own namespace",
			principal: &authn.Principal{Namespace: "ns-admin"},
			result:    result,
			want:      true,
		},
		{
			name:      "other user namespace",
			principal: &authn.Principal{Namespace: "ns-rm68q0bp"},
			result:    result,
			want:      false,
		},
		{
			name:      "system namespace",
			principal: &authn.Principal{Namespace: "kube-system"},
			result:    result,
			want:      false,
		},
		{
			name:      "admin denied",
			principal: &authn.Principal{Namespace: "ns-admin"},
			result:    AdminAuthorizationResult{AllowedNamespace: "ns-admin"},
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isAdminInOwnNamespace(tt.principal, tt.result); got != tt.want {
				t.Fatalf("isAdminInOwnNamespace() = %v, want %v", got, tt.want)
			}
		})
	}
}
