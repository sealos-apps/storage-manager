package viewer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const sealosUserServiceAccountNamespace = "user-system"

type kubernetesAdminAuthorizer struct {
	allowedUserIDs       []string
	recorder             *observability.Recorder
	managementRESTConfig *rest.Config
}

type denyAdminAuthorizer struct{}

type AdminAuthorizationResult struct {
	Allowed            bool
	AllowedNamespace   string
	KubernetesUsername string
	Reason             string
}

func newKubernetesAdminAuthorizer(
	cfg config.AdminConfig,
	recorder *observability.Recorder,
	managementRESTConfig *rest.Config,
) kubernetesAdminAuthorizer {
	return kubernetesAdminAuthorizer{
		allowedUserIDs:       cfg.AllowedUserIDs,
		recorder:             recorder,
		managementRESTConfig: managementRESTConfig,
	}
}

func (denyAdminAuthorizer) CanAdmin(_ context.Context, _ *authn.Principal) (AdminAuthorizationResult, error) {
	return AdminAuthorizationResult{Reason: "not_configured"}, errors.New("admin access denied")
}

func (denyAdminAuthorizer) CanManageStorageClasses(_ context.Context, _ *authn.Principal) error {
	return errors.New("admin access denied")
}

func (a kubernetesAdminAuthorizer) CanAdmin(
	ctx context.Context,
	principal *authn.Principal,
) (result AdminAuthorizationResult, err error) {
	ctx, finish := a.recorder.TraceOperation(ctx, "admin.authorize_storageclasses")
	defer func() {
		finish(err)
	}()

	if len(a.allowedUserIDs) == 0 {
		result.Reason = "no_admin_users_configured"
		return result, errors.New("no admin users configured")
	}
	clientset, err := kubernetesClientsetForConfig(clientConfigForPrincipal(a.managementRESTConfig, principal))
	if err != nil {
		result.Reason = "client_config_error"
		return result, err
	}
	review, err := clientset.AuthenticationV1().SelfSubjectReviews().Create(
		ctx,
		&authenticationv1.SelfSubjectReview{},
		metav1.CreateOptions{},
	)
	if err != nil {
		result.Reason = "self_subject_review_failed"
		return result, err
	}
	username := strings.TrimSpace(review.Status.UserInfo.Username)
	if username == "" {
		result.Reason = "empty_username"
		return result, errors.New("self subject review returned empty username")
	}
	result.KubernetesUsername = username
	a.recorder.Logger().LogAttrs(ctx, slog.LevelDebug, "admin.self_subject_review",
		slog.String("username", username),
	)
	for _, userID := range a.allowedUserIDs {
		if username == sealosAdminUsername(userID) {
			result.Allowed = true
			result.AllowedNamespace = sealosUserNamespace(userID)
			result.Reason = "allowed"
			return result, nil
		}
	}
	result.Reason = "not_admin"
	return result, fmt.Errorf("username %q is not an allowed admin", username)
}

func (a kubernetesAdminAuthorizer) CanManageStorageClasses(ctx context.Context, principal *authn.Principal) error {
	_, err := a.CanAdmin(ctx, principal)
	return err
}

func sealosAdminUsername(userID string) string {
	return "system:serviceaccount:" + sealosUserServiceAccountNamespace + ":" + strings.TrimSpace(userID)
}

func sealosUserNamespace(userID string) string {
	return "ns-" + strings.TrimSpace(userID)
}

func allowedAdminUsernames(userIDs []string) []string {
	usernames := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		username := sealosAdminUsername(userID)
		if !slices.Contains(usernames, username) {
			usernames = append(usernames, username)
		}
	}
	return usernames
}
