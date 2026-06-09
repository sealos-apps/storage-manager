package filebrowser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var defaultTransport = http.DefaultTransport

type Client struct {
	httpClient *http.Client
}

type LoginRequest struct {
	Username string `json:"username"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

func NewClient(timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
	}
}

func NewObservedClient(timeout time.Duration, provider trace.TracerProvider) *Client {
	if provider == nil {
		return NewClient(timeout)
	}
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: otelhttp.NewTransport(
				cloneTransport(defaultTransport),
				otelhttp.WithTracerProvider(provider),
				otelhttp.WithMeterProvider(noop.NewMeterProvider()),
				otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator(
					propagation.TraceContext{},
					propagation.Baggage{},
				)),
				otelhttp.WithSpanNameFormatter(func(_ string, _ *http.Request) string {
					return "filebrowser.http.login"
				}),
			),
		},
	}
}

func cloneTransport(rt http.RoundTripper) http.RoundTripper {
	if transport, ok := rt.(*http.Transport); ok {
		return transport.Clone()
	}
	return rt
}

func (c *Client) Login(ctx context.Context, viewerURL string, username string, password string) (string, error) {
	body, err := json.Marshal(loginRequestBody(username, password))
	if err != nil {
		return "", fmt.Errorf("encoding filebrowser login request: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(viewerURL, "/")+"/api/login",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("building filebrowser login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling filebrowser login: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("filebrowser login returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading filebrowser login response: %w", err)
	}
	token, err := loginToken(data)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", fmt.Errorf("filebrowser login response missing token")
	}
	return token, nil
}

func loginRequestBody(username string, password string) map[string]string {
	return map[string]string{
		"username": username,
		"password": password,
	}
}

func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func loginToken(data []byte) (string, error) {
	var out LoginResponse
	if err := json.Unmarshal(data, &out); err == nil && out.Token != "" {
		return out.Token, nil
	}
	var token string
	if err := json.Unmarshal(data, &token); err == nil {
		return strings.TrimSpace(token), nil
	}
	return strings.TrimSpace(string(data)), nil
}
