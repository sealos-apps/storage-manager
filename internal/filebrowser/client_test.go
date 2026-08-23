package filebrowser

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestLoginToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "json object",
			body: `{"token":"jwt-token"}`,
			want: "jwt-token",
		},
		{
			name: "json string",
			body: `"jwt-token"`,
			want: "jwt-token",
		},
		{
			name: "plain text",
			body: "jwt-token\n",
			want: "jwt-token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loginToken([]byte(tt.body))
			if err != nil {
				t.Fatalf("loginToken() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("loginToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoginSendsCredentialsAsFileBrowserJSON(t *testing.T) {
	t.Parallel()

	var path string
	var body []byte
	client := &Client{loginHTTPClient: &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			path = req.URL.Path
			data, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			body = data
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"token":"jwt-token"}`)),
				Request:    req,
			}, nil
		}),
	}}

	token, err := client.Login(context.Background(), "http://filebrowser.example/", "viewer", "secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if token != "jwt-token" {
		t.Fatalf("token = %q", token)
	}
	if path != "/api/login" {
		t.Fatalf("path = %q", path)
	}
	var request map[string]string
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if request["username"] != "viewer" || request["password"] != "secret" {
		t.Fatalf("request body = %#v", request)
	}
}

func TestProxyReplacesAuthorizationAndDropsBrowserCookies(t *testing.T) {
	t.Parallel()

	var upstream *http.Request
	var body []byte
	client := &Client{proxyHTTPClient: &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			upstream = req
			body, _ = io.ReadAll(req.Body)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		}),
	}}
	source, err := http.NewRequest(http.MethodPost, "http://storage-manager/viewer-files/tus", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("build source request: %v", err)
	}
	source.Header.Set("Authorization", "Bearer browser-auth")
	source.Header.Set("X-Auth", "browser-auth")
	source.Header.Set("Cookie", "storage-manager=session")

	response, err := client.Proxy(context.Background(), "http://filebrowser/api/tus/upload-1", source, "server-token")
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if upstream == nil {
		t.Fatal("upstream request was not captured")
	}
	if upstream.Header.Get("Authorization") != "Bearer server-token" || upstream.Header.Get("X-Auth") != "server-token" {
		t.Fatalf("upstream auth headers = %#v", upstream.Header)
	}
	if upstream.Header.Get("Cookie") != "" {
		t.Fatalf("upstream cookie = %q", upstream.Header.Get("Cookie"))
	}
	if string(body) != "payload" {
		t.Fatalf("upstream body = %q", body)
	}
}

func TestObservedClientInjectsTraceContext(t *testing.T) {
	originalTransport := defaultTransport
	var traceparent string
	defaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		traceparent = req.Header.Get("traceparent")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"token":"jwt-token"}`)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() {
		defaultTransport = originalTransport
	})

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	client := NewObservedClient(0, provider)
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("TraceIDFromHex() error = %v", err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex() error = %v", err)
	}
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	}))
	token, err := client.Login(ctx, "http://filebrowser.example", "viewer", "secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if token != "jwt-token" {
		t.Fatalf("token = %q", token)
	}
	if !strings.HasPrefix(traceparent, "00-4bf92f3577b34da6a3ce929d0e0e4736-") {
		t.Fatalf("traceparent = %q", traceparent)
	}

	carrier := propagation.MapCarrier{"traceparent": traceparent}
	extracted := propagation.TraceContext{}.Extract(context.Background(), carrier)
	if got := trace.SpanContextFromContext(extracted).TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("injected trace id = %s", got)
	}
}

func TestObservedClientUsesLoginTimeoutOnlyForLogin(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	client := NewObservedClient(2*time.Second, provider)
	if got := client.loginHTTPClient.Timeout; got != 2*time.Second {
		t.Fatalf("login timeout = %s, want %s", got, 2*time.Second)
	}
	if got := client.proxyHTTPClient.Timeout; got != 0 {
		t.Fatalf("proxy timeout = %s, want zero", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
