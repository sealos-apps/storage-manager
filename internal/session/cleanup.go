package session

import (
	"context"
	"log/slog"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
)

type CleanupService struct {
	cfg      config.Config
	store    *state.Store
	pods     *PodService
	recorder *observability.Recorder
	now      func() time.Time
}

func NewCleanupService(
	cfg config.Config,
	store *state.Store,
	pods *PodService,
	recorder *observability.Recorder,
) *CleanupService {
	return &CleanupService{
		cfg:      cfg,
		store:    store,
		pods:     pods,
		recorder: recorder,
		now:      time.Now,
	}
}

func (s *CleanupService) RunOnce(ctx context.Context) (err error) {
	ctx, finish := s.recorder.TraceOperation(ctx, "cleanup.run_once")
	var idlePodsDeleted, expiredViewerSessions int
	defer func() {
		s.recorder.Logger().LogAttrs(ctx, slog.LevelDebug, "cleanup.run_once.result",
			slog.Int("idle_pods_deleted", idlePodsDeleted),
			slog.Int("expired_viewer_sessions", expiredViewerSessions),
		)
		finish(err)
	}()

	now := s.now()
	idlePodsDeleted, err = s.cleanupIdlePods(ctx, now)
	if err != nil {
		return err
	}
	expired := s.store.PurgeExpired(now)
	for _, item := range expired {
		if item.Kind == "viewer_session" {
			s.recorder.ObserveViewerSession("closed")
			expiredViewerSessions++
		}
	}
	return nil
}

func (s *CleanupService) cleanupIdlePods(ctx context.Context, now time.Time) (deleted int, err error) {
	for _, podSession := range s.store.ListExpiredPodSessions(now) {
		if len(s.store.ListViewerSessionsByPod(podSession.ID, now)) > 0 {
			podSession.ExpiresAt = now.Add(s.cfg.Sessions.PodKeepaliveGrace)
			podSession.LastActiveAt = now
			podSession.UpdatedAt = now
			s.store.PutPodSession(podSession)
			continue
		}
		if _, err := s.pods.ClosePodSession(ctx, podSession.ID); err != nil {
			return deleted, err
		}
		s.recorder.ObserveCleanupDeleted()
		deleted++
	}
	return deleted, nil
}
