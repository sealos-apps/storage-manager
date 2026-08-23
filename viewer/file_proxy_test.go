package viewer

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
)

type recordingFileBrowserProxy struct {
	calls      int
	lastTarget string
	lastToken  string
	response   *http.Response
}

func (p *recordingFileBrowserProxy) Proxy(_ context.Context, targetURL string, _ *http.Request, token string) (*http.Response, error) {
	p.calls++
	p.lastTarget = targetURL
	p.lastToken = token
	return p.response, nil
}

func newFileProxyTestHandler(proxy fileBrowserProxy) *Handler {
	return NewHandler(
		&fakeViewerService{
			created: &domain.ViewerSession{
				ID:           "vs_1",
				PodSessionID: "ps_1",
				Namespace:    "ns",
				PVCName:      "data",
			},
			token: &domain.ViewerToken{
				ViewerSessionID:   "vs_1",
				PodSessionID:      "ps_1",
				ViewerURL:         "https://viewer.example.test",
				InternalViewerURL: "https://viewer.example.test",
				Token:             "fb",
				ExpiresAt:         time.Now().Add(time.Minute),
			},
		},
		fakePodService{},
		fakeAuthService{},
		nil,
		observability.MustNew(testObservability(), nil),
		allowAuthorizer{},
		WithFileBrowserProxy(proxy),
	)
}

func newFileProxyRequest(method string, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", url.QueryEscape(testKubeconfig))
	return req
}

func TestProxyViewerFilesKeepsFileBrowserTokenServerSide(t *testing.T) {
	proxy := &recordingFileBrowserProxy{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Authorization": {"Bearer fb"},
				"X-Auth":        {"fb"},
				"Set-Cookie":    {"filebrowser=session"},
				"Content-Type":  {"application/octet-stream"},
			},
			Body: io.NopCloser(strings.NewReader("file contents")),
		},
	}
	handler := newFileProxyTestHandler(proxy)
	recorder := httptest.NewRecorder()

	handler.ProxyViewerFiles(
		recorder,
		newFileProxyRequest(http.MethodGet, "/viewer-files/raw?viewer_session_id=vs_1&path=/docs/readme.md"),
		fileProxyRaw,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "file contents" {
		t.Fatalf("body = %q", recorder.Body.String())
	}
	if proxy.lastToken != "fb" {
		t.Fatalf("upstream token = %q", proxy.lastToken)
	}
	if proxy.lastTarget != "https://viewer.example.test/api/raw/docs/readme.md" {
		t.Fatalf("upstream target = %q", proxy.lastTarget)
	}
	for _, header := range []string{"Authorization", "X-Auth", "Set-Cookie"} {
		if value := recorder.Header().Get(header); value != "" {
			t.Fatalf("response leaked %s: %q", header, value)
		}
	}
}

func TestProxyViewerFilesPreservesDirectoryTrailingSlash(t *testing.T) {
	proxy := &recordingFileBrowserProxy{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("{}")),
		},
	}
	handler := newFileProxyTestHandler(proxy)
	recorder := httptest.NewRecorder()

	handler.ProxyViewerFiles(
		recorder,
		newFileProxyRequest(http.MethodPost, "/viewer-files/resources?viewer_session_id=vs_1&path=/docs/"),
		fileProxyResources,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if proxy.lastTarget != "https://viewer.example.test/api/resources/docs/?override=false" {
		t.Fatalf("upstream target = %q", proxy.lastTarget)
	}
}

func TestProxyViewerFilesRewritesTUSLocationToTheProxy(t *testing.T) {
	proxy := &recordingFileBrowserProxy{
		response: &http.Response{
			StatusCode: http.StatusCreated,
			Header:     http.Header{"Location": {"/api/tus/upload-1"}},
			Body:       io.NopCloser(strings.NewReader("")),
		},
	}
	handler := newFileProxyTestHandler(proxy)
	initial := newFileProxyRequest(http.MethodPost, "/viewer-files/tus?viewer_session_id=vs_1&path=/data.bin")
	initialRecorder := httptest.NewRecorder()

	handler.ProxyViewerFiles(initialRecorder, initial, fileProxyTUS)

	if initialRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d", initialRecorder.Code)
	}
	location := initialRecorder.Header().Get("Location")
	parsedLocation, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse proxy location: %v", err)
	}
	if parsedLocation.Path != "/viewer-files/tus" {
		t.Fatalf("proxy location path = %q", parsedLocation.Path)
	}
	if parsedLocation.Query().Get(tusUploadPathQuery) != "/api/tus/upload-1" {
		t.Fatalf("proxy upload path = %q", parsedLocation.Query().Get(tusUploadPathQuery))
	}

	proxy.response = &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
	}
	followUp := newFileProxyRequest(http.MethodPatch, location)
	followUpRecorder := httptest.NewRecorder()
	handler.ProxyViewerFiles(followUpRecorder, followUp, fileProxyTUS)

	if followUpRecorder.Code != http.StatusNoContent {
		t.Fatalf("follow-up status = %d body=%s", followUpRecorder.Code, followUpRecorder.Body.String())
	}
	if proxy.lastTarget != "https://viewer.example.test/api/tus/upload-1" {
		t.Fatalf("follow-up target = %q", proxy.lastTarget)
	}
}

func TestProxyViewerFilesRejectsInvalidRequestsBeforeUpstream(t *testing.T) {
	tests := []struct {
		name   string
		target string
		auth   string
	}{
		{name: "missing session", target: "/viewer-files/raw?path=/data.txt", auth: url.QueryEscape(testKubeconfig)},
		{name: "traversal", target: "/viewer-files/raw?viewer_session_id=vs_1&path=/../etc/passwd", auth: url.QueryEscape(testKubeconfig)},
		{name: "unauthorized", target: "/viewer-files/raw?viewer_session_id=vs_1&path=/data.txt", auth: "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := &recordingFileBrowserProxy{}
			handler := newFileProxyTestHandler(proxy)
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			req.Header.Set("Authorization", tt.auth)
			recorder := httptest.NewRecorder()

			handler.ProxyViewerFiles(recorder, req, fileProxyRaw)

			if recorder.Code < http.StatusBadRequest || recorder.Code >= http.StatusInternalServerError {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			if proxy.calls != 0 {
				t.Fatalf("upstream calls = %d", proxy.calls)
			}
		})
	}
}
