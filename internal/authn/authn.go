package authn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Principal struct {
	ID           string
	ContextName  string
	Namespace    string
	ClientConfig *rest.Config
}

type contextKey struct{}

func DecodeAuthorization(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", errors.New("authorization header is required")
	}
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		header = strings.TrimSpace(header[len("bearer "):])
	}
	decoded, err := url.QueryUnescape(header)
	if err != nil {
		return "", fmt.Errorf("decoding kubeconfig authorization: %w", err)
	}
	if strings.TrimSpace(decoded) == "" {
		return "", errors.New("decoded kubeconfig is empty")
	}
	return decoded, nil
}

func PrincipalFromAuthorization(header string) (*Principal, error) {
	raw, err := DecodeAuthorization(header)
	if err != nil {
		return nil, err
	}

	apiConfig, err := clientcmd.Load([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}
	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("building rest config from kubeconfig: %w", err)
	}

	contextName := apiConfig.CurrentContext
	namespace := "default"
	if ctx, ok := apiConfig.Contexts[contextName]; ok && strings.TrimSpace(ctx.Namespace) != "" {
		namespace = ctx.Namespace
	}

	return &Principal{
		ID:           SafeIDFromKubeconfig(raw),
		ContextName:  contextName,
		Namespace:    namespace,
		ClientConfig: restConfig,
	}, nil
}

func WithPrincipal(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(*Principal)
	return principal, ok
}

func SafeIDFromKubeconfig(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:8])
}
