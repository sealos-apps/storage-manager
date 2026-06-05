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
)

const sealosUserServiceAccountNamespace = "user-system"

type kubernetesAdminAuthorizer struct {
	allowedUserIDs []string
	recorder       *observability.Recorder
}

type denyAdminAuthorizer struct{}

func newKubernetesAdminAuthorizer(cfg config.AdminConfig, recorder *observability.Recorder) kubernetesAdminAuthorizer {
	return kubernetesAdminAuthorizer{
		allowedUserIDs: cfg.AllowedUserIDs,
		recorder:       recorder,
	}
}

func (denyAdminAuthorizer) CanManageStorageClasses(_ context.Context, _ *authn.Principal) error {
	return errors.New("admin access denied")
}

func (a kubernetesAdminAuthorizer) CanManageStorageClasses(
	ctx context.Context,
	principal *authn.Principal,
) (err error) {
	ctx, finish := a.recorder.TraceOperation(ctx, "admin.authorize_storageclasses")
	defer func() {
		finish(err)
	}()

	if len(a.allowedUserIDs) == 0 {
		return errors.New("no admin users configured")
	}
	clientset, err := kubernetesClientsetForConfig(principal.ClientConfig)
	if err != nil {
		return err
	}
	review, err := clientset.AuthenticationV1().SelfSubjectReviews().Create(
		ctx,
		&authenticationv1.SelfSubjectReview{},
		metav1.CreateOptions{},
	)
	if err != nil {
		return err
	}
	username := strings.TrimSpace(review.Status.UserInfo.Username)
	if username == "" {
		return errors.New("self subject review returned empty username")
	}
	a.recorder.Logger().LogAttrs(ctx, slog.LevelDebug, "admin.self_subject_review",
		slog.String("username", username),
	)
	for _, userID := range a.allowedUserIDs {
		if username == sealosAdminUsername(userID) {
			return nil
		}
	}
	return fmt.Errorf("username %q is not an allowed admin", username)
}

func sealosAdminUsername(userID string) string {
	return "system:serviceaccount:" + sealosUserServiceAccountNamespace + ":" + strings.TrimSpace(userID)
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
