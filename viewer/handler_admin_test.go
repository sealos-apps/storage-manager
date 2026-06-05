package viewer

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandlerAdminCapabilitiesReturnsFalseForNonAdmin(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(denyTestAdminAuthorizer{}),
	)
	req := httptest.NewRequest(http.MethodGet, "/admin/capabilities", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.AdminCapabilities(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"can_manage_storage_classes":false`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerAdminListStorageClassesRequiresAdmin(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(denyTestAdminAuthorizer{}),
		WithStorageClassService(fakeStorageClassService{}),
	)
	req := httptest.NewRequest(http.MethodGet, "/admin/storage-classes", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.AdminListStorageClasses(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), string(apienv.CodeAdminAccessDenied)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerAdminStorageClassEndpointsUseEnvelope(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
		WithStorageClassService(fakeStorageClassService{
			items:    []domain.StorageClass{{Name: "standard", Provisioner: "test"}},
			item:     &domain.StorageClass{Name: "standard", Provisioner: "test"},
			yaml:     &session.StorageClassYAML{Name: "standard", YAML: "kind: StorageClass\n"},
			describe: &session.StorageClassDescribe{Name: "standard", Describe: "Name: standard"},
		}),
	)
	tests := []struct {
		name   string
		req    *http.Request
		handle func(*Handler, http.ResponseWriter, *http.Request)
		want   string
	}{
		{
			name:   "list",
			req:    httptest.NewRequest(http.MethodGet, "/admin/storage-classes", nil),
			handle: (*Handler).AdminListStorageClasses,
			want:   "storage_class_list",
		},
		{
			name:   "yaml",
			req:    httptest.NewRequest(http.MethodGet, "/admin/storage-classes/standard/yaml", nil),
			handle: (*Handler).AdminGetStorageClassYAML,
			want:   "storage_class_yaml",
		},
		{
			name:   "create",
			req:    httptest.NewRequest(http.MethodPost, "/admin/storage-classes", strings.NewReader(`{"yaml":"kind: StorageClass\n"}`)),
			handle: (*Handler).AdminCreateStorageClass,
			want:   "storage_class",
		},
		{
			name:   "update",
			req:    httptest.NewRequest(http.MethodPut, "/admin/storage-classes/standard", strings.NewReader(`{"yaml":"kind: StorageClass\n"}`)),
			handle: (*Handler).AdminUpdateStorageClass,
			want:   "storage_class",
		},
		{
			name:   "update policy",
			req:    httptest.NewRequest(http.MethodPut, "/admin/storage-classes/standard/policy", strings.NewReader(`{"visible_in_create":true,"allowed_access_modes":["ReadWriteOnce","ReadWriteMany"]}`)),
			handle: (*Handler).AdminUpdateStorageClassPolicy,
			want:   "storage_class",
		},
		{
			name:   "delete",
			req:    httptest.NewRequest(http.MethodDelete, "/admin/storage-classes/standard", nil),
			handle: (*Handler).AdminDeleteStorageClass,
			want:   "storage_class",
		},
		{
			name:   "describe",
			req:    httptest.NewRequest(http.MethodGet, "/admin/storage-classes/standard/describe", nil),
			handle: (*Handler).AdminDescribeStorageClass,
			want:   "storage_class_describe",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
			recorder := httptest.NewRecorder()

			tt.handle(handler, recorder, tt.req)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tt.want) {
				t.Fatalf("body = %s", recorder.Body.String())
			}
		})
	}
}

func TestHandlerAdminListNamespacesFiltersUserNamespaces(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			namespaces: []corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns-other-user"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "sealos-storage-manager"}},
			},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
	)
	req := httptest.NewRequest(http.MethodGet, "/admin/namespaces", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.AdminListNamespaces(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"name":"ns","is_current_context":true`) {
		t.Fatalf("current namespace missing: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"name":"kube-system"`) {
		t.Fatalf("system namespace missing: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "ns-other-user") {
		t.Fatalf("user namespace leaked: %s", recorder.Body.String())
	}
}

func TestHandlerAdminListNamespacesRequiresAdmin(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(denyTestAdminAuthorizer{}),
	)
	req := httptest.NewRequest(http.MethodGet, "/admin/namespaces", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.AdminListNamespaces(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), string(apienv.CodeAdminAccessDenied)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerAdminImplicitPVCOperationsUseRequestedSystemNamespace(t *testing.T) {
	t.Parallel()

	var createInput session.CreatePVCInput
	var expandInput session.ExpandPVCInput
	var deleteInput session.DeletePVCInput
	viewers := &fakeViewerService{
		namespaces: []corev1.Namespace{
			{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		},
		pvcs:        []domain.PVC{{Name: "data", UID: "uid", MountedPods: []domain.MountedPod{}}},
		pvc:         &domain.PVC{Namespace: "kube-system", Name: "data"},
		pvcInput:    &createInput,
		expandInput: &expandInput,
		deleteInput: &deleteInput,
	}
	handler := NewHandler(
		viewers,
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
	)
	tests := []struct {
		name   string
		req    *http.Request
		handle func(*Handler, http.ResponseWriter, *http.Request)
	}{
		{
			name:   "list",
			req:    httptest.NewRequest(http.MethodGet, "/pvcs?namespace=kube-system", nil),
			handle: (*Handler).ListPVCs,
		},
		{
			name: "create",
			req: httptest.NewRequest(
				http.MethodPost,
				"/pvcs",
				strings.NewReader(`{"namespace":"kube-system","name":"data","capacity":"10Gi","access_modes":["ReadWriteOnce"],"storage_class_name":"standard"}`),
			),
			handle: (*Handler).CreatePVC,
		},
		{
			name: "expand",
			req: httptest.NewRequest(
				http.MethodPost,
				"/pvcs/kube-system/data/expand",
				strings.NewReader(`{"capacity":"20Gi"}`),
			),
			handle: (*Handler).ExpandPVC,
		},
		{
			name:   "delete",
			req:    httptest.NewRequest(http.MethodDelete, "/pvcs/kube-system/data", nil),
			handle: (*Handler).DeletePVC,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
			recorder := httptest.NewRecorder()

			tt.handle(handler, recorder, tt.req)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if createInput.Namespace != "kube-system" {
		t.Fatalf("create namespace = %q", createInput.Namespace)
	}
	if expandInput.Namespace != "kube-system" {
		t.Fatalf("expand namespace = %q", expandInput.Namespace)
	}
	if deleteInput.Namespace != "kube-system" {
		t.Fatalf("delete namespace = %q", deleteInput.Namespace)
	}
}

func TestHandlerAdminImplicitPVCRejectsUserNamespace(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			namespaces: []corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns-other-user"}},
			},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
	)
	req := httptest.NewRequest(http.MethodGet, "/pvcs?namespace=ns-other-user", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ListPVCs(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), string(apienv.CodePVCAccessDenied)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerAdminViewerSessionMarksAdminContext(t *testing.T) {
	t.Parallel()

	var input session.CreateViewerSessionInput
	handler := NewHandler(
		&fakeViewerService{
			namespaces: []corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
			},
			created: &domain.ViewerSession{
				ID:           "vs_1",
				PodSessionID: "ps_1",
				Namespace:    "kube-system",
				PVCName:      "data",
				AdminContext: true,
			},
			createInput: &input,
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/viewer-sessions",
		strings.NewReader(`{"namespace":"kube-system","pvc_name":"data"}`),
	)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.CreateViewerSession(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !input.AdminContext {
		t.Fatalf("AdminContext = false")
	}
	if input.Namespace != "kube-system" {
		t.Fatalf("namespace = %q", input.Namespace)
	}
}

func TestHandlerAdminUpdateStorageClassPolicyMapsRequest(t *testing.T) {
	t.Parallel()

	var policyInput session.StorageClassPolicyInput
	var policyName string
	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
		WithStorageClassService(fakeStorageClassService{
			item:        &domain.StorageClass{Name: "standard", Provisioner: "test"},
			policyInput: &policyInput,
			policyName:  &policyName,
		}),
	)
	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/storage-classes/standard/policy",
		strings.NewReader(`{"visible_in_create":true,"allowed_access_modes":["ReadWriteOnce","ReadWriteMany"]}`),
	)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.AdminUpdateStorageClassPolicy(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if policyName != "standard" {
		t.Fatalf("policyName = %q", policyName)
	}
	if !policyInput.VisibleInCreate || strings.Join(policyInput.AllowedAccessModes, ",") != "ReadWriteOnce,ReadWriteMany" {
		t.Fatalf("policyInput = %#v", policyInput)
	}
}

func TestAllowedAdminUsernamesUseSealosServiceAccountShape(t *testing.T) {
	t.Parallel()

	got := allowedAdminUsernames([]string{"admin", " ", "admin", "b4hw543c"})
	want := []string{
		"system:serviceaccount:user-system:admin",
		"system:serviceaccount:user-system:b4hw543c",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("usernames = %#v, want %#v", got, want)
	}
}
