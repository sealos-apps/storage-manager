package viewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
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
