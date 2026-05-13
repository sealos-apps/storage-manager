package authn

import (
	"net/url"
	"strings"
	"testing"
)

const testKubeconfig = `apiVersion: v1
kind: Config
current-context: dev
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
users:
- name: u
  user:
    token: test-token
contexts:
- name: dev
  context:
    cluster: c
    user: u
    namespace: ns
`

func TestDecodeAuthorization(t *testing.T) {
	t.Parallel()

	encoded := url.QueryEscape(testKubeconfig)
	tests := []struct {
		name   string
		header string
	}{
		{name: "raw encoded", header: encoded},
		{name: "bearer encoded", header: "Bearer " + encoded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeAuthorization(tt.header)
			if err != nil {
				t.Fatalf("DecodeAuthorization() error = %v", err)
			}
			if got != testKubeconfig {
				t.Fatal("decoded kubeconfig mismatch")
			}
		})
	}
}

func TestDecodeAuthorizationRejectsBadValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
	}{
		{name: "empty", header: ""},
		{name: "bad escape", header: "%zz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeAuthorization(tt.header); err == nil {
				t.Fatal("DecodeAuthorization() error = nil")
			}
		})
	}
}

func TestPrincipalFromAuthorization(t *testing.T) {
	t.Parallel()

	principal, err := PrincipalFromAuthorization(url.QueryEscape(testKubeconfig))
	if err != nil {
		t.Fatalf("PrincipalFromAuthorization() error = %v", err)
	}
	if principal.Namespace != "ns" {
		t.Fatalf("namespace = %q", principal.Namespace)
	}
	if principal.ContextName != "dev" {
		t.Fatalf("context = %q", principal.ContextName)
	}
	if principal.ID == "" || strings.Contains(principal.ID, "test-token") {
		t.Fatalf("principal id is unsafe: %q", principal.ID)
	}
}
