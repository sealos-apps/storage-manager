package session

import (
	"context"
	"time"

	"github.com/nixieboluo/sealos-stroage-manager/internal/config"
	"github.com/nixieboluo/sealos-stroage-manager/internal/observability"
	"github.com/nixieboluo/sealos-stroage-manager/internal/state"
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

func (s *CleanupService) RunOnce(ctx context.Context) error {
	now := s.now()
	if err := s.cleanupIdlePods(ctx, now); err != nil {
		return err
	}
	expired := s.store.PurgeExpired(now)
	for _, item := range expired {
		if item.Kind == "viewer_session" {
			s.recorder.Metrics().ViewerClosed.Add(1)
		}
	}
	return nil
}

func (s *CleanupService) cleanupIdlePods(ctx context.Context, now time.Time) error {
	for _, podSession := range s.store.ListExpiredPodSessions(now) {
		if len(s.store.ListViewerSessionsByPod(podSession.ID, now)) > 0 {
			podSession.ExpiresAt = now.Add(s.cfg.Sessions.PodKeepaliveGrace)
			podSession.LastActiveAt = now
			podSession.UpdatedAt = now
			s.store.PutPodSession(podSession)
			continue
		}
		if _, err := s.pods.ClosePodSession(ctx, podSession.ID); err != nil {
			return err
		}
		s.recorder.Metrics().CleanupDeleted.Add(1)
	}
	return nil
}
