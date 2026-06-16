package viewer

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
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
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
	recorder := httptest.NewRecorder()

	handler.AdminCapabilities(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"can_manage_storage_classes":false`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"file_management_enabled":true`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"pvc_creation_enabled":true`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"user_namespace":"ns-admin"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerAdminCapabilitiesReportsDisabledFeatures(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Viewer.FileManagement.Enabled = false
	cfg.Viewer.PVCCreation.Enabled = false
	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
		WithFeatureConfig(cfg.Features()),
	)
	req := httptest.NewRequest(http.MethodGet, "/admin/capabilities", nil)
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
	recorder := httptest.NewRecorder()

	handler.AdminCapabilities(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"can_manage_storage_classes":true`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"file_management_enabled":false`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"pvc_creation_enabled":false`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerAdminCapabilitiesRequireOwnNamespaceContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kubeconfig string
	}{
		{name: "system namespace", kubeconfig: testSystemNamespaceKubeconfig},
		{name: "other user namespace", kubeconfig: testOtherUserNamespaceKubeconfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := NewHandler(
				&fakeViewerService{},
				fakePodService{},
				fakeAuthService{},
				nil,
				observability.MustNew(testObservability(), nil),
				allowAuthorizer{},
				WithAdminAuthorizer(allowAdminAuthorizer{}),
			)
			req := httptest.NewRequest(http.MethodGet, "/admin/capabilities", nil)
			req.Header.Set("Authorization", url.QueryEscape(tt.kubeconfig))
			recorder := httptest.NewRecorder()

			handler.AdminCapabilities(recorder, req)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"can_manage_pvcs":false`) {
				t.Fatalf("body = %s", recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"can_manage_storage_classes":false`) {
				t.Fatalf("body = %s", recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"user_namespace":"ns-admin"`) {
				t.Fatalf("body = %s", recorder.Body.String())
			}
		})
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

func TestHandlerAdminListStorageClassesRequiresOwnNamespaceContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kubeconfig string
	}{
		{name: "system namespace", kubeconfig: testSystemNamespaceKubeconfig},
		{name: "other user namespace", kubeconfig: testOtherUserNamespaceKubeconfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := NewHandler(
				&fakeViewerService{},
				fakePodService{},
				fakeAuthService{},
				nil,
				observability.MustNew(testObservability(), nil),
				allowAuthorizer{},
				WithAdminAuthorizer(allowAdminAuthorizer{}),
				WithStorageClassService(fakeStorageClassService{}),
			)
			req := httptest.NewRequest(http.MethodGet, "/admin/storage-classes", nil)
			req.Header.Set("Authorization", url.QueryEscape(tt.kubeconfig))
			recorder := httptest.NewRecorder()

			handler.AdminListStorageClasses(recorder, req)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), string(apienv.CodeAdminAccessDenied)) {
				t.Fatalf("body = %s", recorder.Body.String())
			}
		})
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

			tt.req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
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
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
	recorder := httptest.NewRecorder()

	handler.AdminListNamespaces(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"name":"ns-admin","is_current_context":true`) {
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

func TestHandlerAdminListNamespacesRequiresOwnNamespaceContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kubeconfig string
	}{
		{name: "system namespace", kubeconfig: testSystemNamespaceKubeconfig},
		{name: "other user namespace", kubeconfig: testOtherUserNamespaceKubeconfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := NewHandler(
				&fakeViewerService{
					namespaces: []corev1.Namespace{
						{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
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
			req.Header.Set("Authorization", url.QueryEscape(tt.kubeconfig))
			recorder := httptest.NewRecorder()

			handler.AdminListNamespaces(recorder, req)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), string(apienv.CodeAdminAccessDenied)) {
				t.Fatalf("body = %s", recorder.Body.String())
			}
		})
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
			tt.req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
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

func TestHandlerAdminListAllPVCsAggregatesAllowedNamespaces(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			namespaces: []corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "z-system"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns-other-user"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
			},
			pvcsByNamespace: map[string][]domain.PVC{
				"ns-admin": {
					{Name: "user-data", UID: "uid-user", MountedPods: []domain.MountedPod{}},
				},
				"kube-system": {
					{Name: "kube-data", UID: "uid-kube", MountedPods: []domain.MountedPod{}},
				},
				"z-system": {
					{Name: "z-data", UID: "uid-z", MountedPods: []domain.MountedPod{}},
				},
			},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
	)
	req := httptest.NewRequest(http.MethodGet, "/pvcs?namespace=__all__", nil)
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ListPVCs(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{
		`"namespace":"kube-system","name":"kube-data"`,
		`"namespace":"ns-admin","name":"user-data"`,
		`"namespace":"z-system","name":"z-data"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in body %s", want, body)
		}
	}
	if strings.Contains(body, "ns-other-user") {
		t.Fatalf("user namespace leaked: %s", body)
	}
	kubeIndex := strings.Index(body, `"namespace":"kube-system"`)
	userIndex := strings.Index(body, `"namespace":"ns-admin"`)
	zIndex := strings.Index(body, `"namespace":"z-system"`)
	if kubeIndex >= userIndex || userIndex >= zIndex {
		t.Fatalf("PVCs are not sorted by namespace/name: %s", body)
	}
}

func TestHandlerAdminListAllPVCsRequiresAdmin(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/pvcs?namespace=__all__", nil)
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ListPVCs(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), string(apienv.CodeAdminAccessDenied)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerAdminImplicitPVCRequiresOwnNamespaceContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kubeconfig string
	}{
		{name: "other user namespace", kubeconfig: testOtherUserNamespaceKubeconfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := NewHandler(
				&fakeViewerService{
					namespaces: []corev1.Namespace{
						{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
					},
				},
				fakePodService{},
				fakeAuthService{},
				nil,
				observability.MustNew(testObservability(), nil),
				allowAuthorizer{},
				WithAdminAuthorizer(allowAdminAuthorizer{}),
			)
			req := httptest.NewRequest(http.MethodGet, "/pvcs?namespace=kube-system", nil)
			req.Header.Set("Authorization", url.QueryEscape(tt.kubeconfig))
			recorder := httptest.NewRecorder()

			handler.ListPVCs(recorder, req)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), string(apienv.CodeAdminAccessDenied)) {
				t.Fatalf("body = %s", recorder.Body.String())
			}
		})
	}
}

func TestHandlerAdminCurrentContextPVCUsesUserAuthorization(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		denyAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
	)
	req := httptest.NewRequest(http.MethodGet, "/pvcs?namespace=kube-system", nil)
	req.Header.Set("Authorization", url.QueryEscape(testSystemNamespaceKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ListPVCs(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), string(apienv.CodePVCAccessDenied)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerAdminCanListJoinedNamespaceFromJoinedContext(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			pvcs: []domain.PVC{{Namespace: "ns-rm68q0bp", Name: "joined-data", MountedPods: []domain.MountedPod{}}},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
	)
	req := httptest.NewRequest(http.MethodGet, "/pvcs?namespace=ns-rm68q0bp", nil)
	req.Header.Set("Authorization", url.QueryEscape(testOtherUserNamespaceKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ListPVCs(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"namespace":"ns-rm68q0bp"`) {
		t.Fatalf("body = %s", recorder.Body.String())
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
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
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
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
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
