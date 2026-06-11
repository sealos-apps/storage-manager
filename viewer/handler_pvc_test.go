package viewer

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/accountquota"
	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/session"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandlerListPVCsUsesEnvelope(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			pvcs: []domain.PVC{{Namespace: "ns", Name: "data", MountedPods: []domain.MountedPod{}}},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(http.MethodGet, "/pvcs?namespace=ns", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ListPVCs(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]struct {
		Items []domain.PVC `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body["pvc_list"].Items) != 1 {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerGetContextUsesKubeconfigNamespace(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(http.MethodGet, "/context", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.GetContext(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"namespace":"ns"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerGetContextUsesDebugForcedNamespace(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithDebugConfig(config.DebugConfig{
			Enabled:         true,
			ForcedNamespace: "forced-ns",
		}),
	)
	req := httptest.NewRequest(http.MethodGet, "/context", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.GetContext(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"namespace":"forced-ns"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerForcedNamespaceRejectsKubeconfigNamespaceRequest(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithDebugConfig(config.DebugConfig{
			Enabled:         true,
			ForcedNamespace: "forced-ns",
		}),
	)
	req := httptest.NewRequest(http.MethodGet, "/pvcs?namespace=ns", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ListPVCs(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerRejectsExplicitDifferentNamespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request func() *http.Request
		handle  func(*Handler, http.ResponseWriter, *http.Request)
	}{
		{
			name: "list pvcs",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/pvcs?namespace=other", nil)
			},
			handle: (*Handler).ListPVCs,
		},
		{
			name: "create pvc",
			request: func() *http.Request {
				return httptest.NewRequest(
					http.MethodPost,
					"/pvcs",
					strings.NewReader(`{"namespace":"other","name":"data","capacity":"10Gi","access_modes":["ReadWriteOnce"]}`),
				)
			},
			handle: (*Handler).CreatePVC,
		},
		{
			name: "expand pvc",
			request: func() *http.Request {
				return httptest.NewRequest(
					http.MethodPost,
					"/pvcs/other/data/expand",
					strings.NewReader(`{"capacity":"20Gi"}`),
				)
			},
			handle: (*Handler).ExpandPVC,
		},
		{
			name: "delete pvc",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodDelete, "/pvcs/other/data", nil)
			},
			handle: (*Handler).DeletePVC,
		},
		{
			name: "create viewer session",
			request: func() *http.Request {
				return httptest.NewRequest(
					http.MethodPost,
					"/viewer-sessions",
					strings.NewReader(`{"namespace":"other","pvc_name":"data"}`),
				)
			},
			handle: (*Handler).CreateViewerSession,
		},
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
			)
			req := tt.request()
			req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
			recorder := httptest.NewRecorder()

			tt.handle(handler, recorder, req)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestHandlerCreatePVCUsesEnvelope(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			pvc: &domain.PVC{Namespace: "ns", Name: "data", Capacity: "10Gi"},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/pvcs",
		strings.NewReader(`{"namespace":"ns","name":"data","capacity":"10Gi","access_modes":["ReadWriteOnce"]}`),
	)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.CreatePVC(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"pvc"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerCreatePVCRejectsWhenFeatureDisabled(t *testing.T) {
	t.Parallel()

	var input session.CreatePVCInput
	cfg := config.Default()
	cfg.Viewer.PVCCreation.Enabled = false
	handler := NewHandler(
		&fakeViewerService{
			pvc:      &domain.PVC{Namespace: "ns", Name: "data", Capacity: "10Gi"},
			pvcInput: &input,
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithFeatureConfig(cfg.Features()),
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/pvcs",
		strings.NewReader(`{"namespace":"ns","name":"data","capacity":"10Gi","access_modes":["ReadWriteOnce"]}`),
	)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.CreatePVC(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), string(apienv.CodePVCCreateForbidden)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if input.Name != "" {
		t.Fatalf("CreatePVC input = %#v", input)
	}
}

func TestHandlerGetStorageQuotaUsesAccountServiceQuota(t *testing.T) {
	t.Parallel()

	var input storageQuotaCall
	handler := NewHandler(
		&fakeViewerService{},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithStorageQuotaService(fakeStorageQuotaService{
			input: &input,
			response: accountquota.StorageQuota{
				AvailableBytes:    15 * 1024 * 1024 * 1024,
				AvailableQuantity: "15Gi",
				LimitBytes:        20 * 1024 * 1024 * 1024,
				LimitQuantity:     "20Gi",
				UsedBytes:         5 * 1024 * 1024 * 1024,
				UsedQuantity:      "5Gi",
			},
		}),
	)
	req := httptest.NewRequest(http.MethodGet, "/storage-quota", nil)
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
	req.Header.Set("X-Sealos-Account-Authorization", "Bearer account.jwt.token")
	recorder := httptest.NewRecorder()

	handler.GetStorageQuota(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if input.namespace != "ns-admin" {
		t.Fatalf("quota namespace = %q", input.namespace)
	}
	if input.authorization != "Bearer account.jwt.token" {
		t.Fatalf("quota authorization = %q", input.authorization)
	}
	if !strings.Contains(recorder.Body.String(), `"available_quantity":"15Gi"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerGetStorageQuotaUsesFixedLimitForSystemNamespace(t *testing.T) {
	t.Parallel()

	var calls int
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
		WithFeatureConfig(config.FeatureConfig{
			StorageQuota: config.StorageQuotaConfig{
				SystemQuota: "200Gi",
			},
		}),
		WithStorageQuotaService(fakeStorageQuotaService{
			calls: &calls,
			err:   errors.New("account service should not be called for system namespaces"),
		}),
	)
	req := httptest.NewRequest(http.MethodGet, "/storage-quota?namespace=kube-system", nil)
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
	recorder := httptest.NewRecorder()

	handler.GetStorageQuota(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if calls != 0 {
		t.Fatalf("storage quota calls = %d", calls)
	}
	if !strings.Contains(recorder.Body.String(), `"available_quantity":"200Gi"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerCreatePVCRejectsWhenStorageQuotaExceeded(t *testing.T) {
	t.Parallel()

	var input session.CreatePVCInput
	handler := NewHandler(
		&fakeViewerService{
			pvc:      &domain.PVC{Namespace: "ns-admin", Name: "data", Capacity: "10Gi"},
			pvcInput: &input,
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithStorageQuotaService(fakeStorageQuotaService{
			response: accountquota.StorageQuota{
				AvailableBytes:    5 * 1024 * 1024 * 1024,
				AvailableQuantity: "5Gi",
			},
		}),
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/pvcs",
		strings.NewReader(`{"namespace":"ns-admin","name":"data","capacity":"10Gi","access_modes":["ReadWriteOnce"]}`),
	)
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
	recorder := httptest.NewRecorder()

	handler.CreatePVC(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), string(apienv.CodePVCQuotaExceeded)) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if input.Name != "" {
		t.Fatalf("CreatePVC input = %#v", input)
	}
}

func TestHandlerCreatePVCInSystemNamespaceUsesFixedStorageQuota(t *testing.T) {
	t.Parallel()

	var calls int
	var input session.CreatePVCInput
	handler := NewHandler(
		&fakeViewerService{
			namespaces: []corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
			},
			pvc:      &domain.PVC{Namespace: "kube-system", Name: "data", Capacity: "10Gi"},
			pvcInput: &input,
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithAdminAuthorizer(allowAdminAuthorizer{}),
		WithStorageQuotaService(fakeStorageQuotaService{
			calls: &calls,
			err:   errors.New("account service should not be called for system namespaces"),
		}),
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/pvcs",
		strings.NewReader(`{"namespace":"kube-system","name":"data","capacity":"10Gi","access_modes":["ReadWriteOnce"],"storage_class_name":"standard"}`),
	)
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
	recorder := httptest.NewRecorder()

	handler.CreatePVC(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if calls != 0 {
		t.Fatalf("storage quota calls = %d", calls)
	}
	if input.Namespace != "kube-system" {
		t.Fatalf("CreatePVC input = %#v", input)
	}
}

func TestHandlerExpandPVCChecksOnlyStorageQuotaDelta(t *testing.T) {
	t.Parallel()

	var input session.ExpandPVCInput
	handler := NewHandler(
		&fakeViewerService{
			pvcs: []domain.PVC{{
				Namespace:     "ns-admin",
				Name:          "data",
				CapacityBytes: 10 * 1024 * 1024 * 1024,
			}},
			pvc:         &domain.PVC{Namespace: "ns-admin", Name: "data", Capacity: "20Gi"},
			expandInput: &input,
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithStorageQuotaService(fakeStorageQuotaService{
			response: accountquota.StorageQuota{
				AvailableBytes:    12 * 1024 * 1024 * 1024,
				AvailableQuantity: "12Gi",
			},
		}),
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/pvcs/ns-admin/data/expand",
		strings.NewReader(`{"capacity":"20Gi","capacity_bytes":21474836480}`),
	)
	req.Header.Set("Authorization", url.QueryEscape(testUserNamespaceKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ExpandPVC(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if input.CapacityBytes != 20*1024*1024*1024 {
		t.Fatalf("ExpandPVC input = %#v", input)
	}
}

func TestHandlerExpandPVCUsesPathParams(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			pvc: &domain.PVC{Namespace: "ns", Name: "data", Capacity: "20Gi"},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/pvcs/ns/data/expand",
		strings.NewReader(`{"capacity":"20Gi"}`),
	)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ExpandPVC(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"capacity":"20Gi"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandlerListStorageClassesUsesEnvelope(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		&fakeViewerService{
			storageClasses: []domain.StorageClass{{Name: "standard", Provisioner: "test"}},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
	)
	req := httptest.NewRequest(http.MethodGet, "/storage-classes", nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	recorder := httptest.NewRecorder()

	handler.ListStorageClasses(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"storage_class_list"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}
