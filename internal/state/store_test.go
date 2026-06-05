package state

import (
	"sync"
	"testing"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
)

func TestStorePodSessionByPVC(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	store := New(config.Default().Cache)
	store.PutPodSession(&domain.PodSession{
		ID:        "ps_1",
		Namespace: "default",
		PVCUID:    "uid",
		ExpiresAt: now.Add(time.Minute),
	})

	got, ok := store.FindPodSessionByPVC("default", "uid", now)
	if !ok {
		t.Fatal("FindPodSessionByPVC() ok = false")
	}
	if got.ID != "ps_1" {
		t.Fatalf("pod session id = %q", got.ID)
	}

	got.ID = "mutated"
	again, ok := store.GetPodSession("ps_1", now)
	if !ok || again.ID != "ps_1" {
		t.Fatalf("store returned mutable pointer: %#v ok=%v", again, ok)
	}
}

func TestStorePodSessionByPVCReplacesStaleIndex(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	store := New(config.Default().Cache)
	store.PutPodSession(&domain.PodSession{
		ID:        "ps_1",
		Namespace: "default",
		PVCUID:    "old",
		ExpiresAt: now.Add(time.Minute),
	})
	store.PutPodSession(&domain.PodSession{
		ID:        "ps_1",
		Namespace: "default",
		PVCUID:    "new",
		ExpiresAt: now.Add(time.Minute),
	})

	if _, ok := store.FindPodSessionByPVC("default", "old", now); ok {
		t.Fatal("FindPodSessionByPVC returned stale PVC index")
	}
	got, ok := store.FindPodSessionByPVC("default", "new", now)
	if !ok || got.ID != "ps_1" {
		t.Fatalf("FindPodSessionByPVC(new) = %#v ok=%v", got, ok)
	}
}

func TestStoreExpiresViewerSessionsAndMapping(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	store := New(config.Default().Cache)
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_1",
		PodSessionID: "ps_1",
		ExpiresAt:    now.Add(time.Second),
	})

	if got := store.ListViewerSessionsByPod("ps_1", now); len(got) != 1 {
		t.Fatalf("sessions before expiry = %d", len(got))
	}
	expired := store.PurgeExpired(now.Add(2 * time.Second))
	if len(expired) != 1 || expired[0].Kind != "viewer_session" {
		t.Fatalf("expired = %#v", expired)
	}
	if got := store.ListViewerSessionsByPod("ps_1", now.Add(2*time.Second)); len(got) != 0 {
		t.Fatalf("sessions after expiry = %d", len(got))
	}
}

func TestStoreViewerSessionByPodReplacesStaleIndex(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	store := New(config.Default().Cache)
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_1",
		PodSessionID: "ps_old",
		ExpiresAt:    now.Add(time.Minute),
	})
	store.PutViewerSession(&domain.ViewerSession{
		ID:           "vs_1",
		PodSessionID: "ps_new",
		ExpiresAt:    now.Add(time.Minute),
	})

	if got := store.ListViewerSessionsByPod("ps_old", now); len(got) != 0 {
		t.Fatalf("old pod sessions = %d", len(got))
	}
	got := store.ListViewerSessionsByPod("ps_new", now)
	if len(got) != 1 || got[0].ID != "vs_1" {
		t.Fatalf("new pod sessions = %#v", got)
	}
}

func TestStoreLRUEvictsOldest(t *testing.T) {
	t.Parallel()

	cfg := config.Default().Cache
	cfg.PodSessionsMaxEntries = 1
	now := time.Now()
	store := New(cfg)
	store.PutPodSession(&domain.PodSession{ID: "old", Namespace: "n", PVCUID: "old", ExpiresAt: now.Add(time.Hour)})
	store.PutPodSession(&domain.PodSession{ID: "new", Namespace: "n", PVCUID: "new", ExpiresAt: now.Add(time.Hour)})

	if _, ok := store.GetPodSession("old", now); ok {
		t.Fatal("old pod session was not evicted")
	}
	if _, ok := store.GetPodSession("new", now); !ok {
		t.Fatal("new pod session missing")
	}
}

func TestListPodSessionsReturnsClones(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := New(config.Default().Cache)
	store.PutPodSession(&domain.PodSession{
		ID:        "ps_1",
		Namespace: "default",
		PVCUID:    "uid",
		ExpiresAt: now.Add(time.Hour),
	})
	sessions := store.ListPodSessions(now)
	if len(sessions) != 1 {
		t.Fatalf("pod sessions = %d", len(sessions))
	}
	sessions[0].ID = "mutated"
	again, ok := store.GetPodSession("ps_1", now)
	if !ok || again.ID != "ps_1" {
		t.Fatalf("store returned mutable pointer: %#v ok=%v", again, ok)
	}
}

func TestListExpiredPodSessionsReturnsExpiredClones(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := New(config.Default().Cache)
	store.PutPodSession(&domain.PodSession{
		ID:        "expired",
		Namespace: "default",
		PVCUID:    "expired",
		ExpiresAt: now.Add(-time.Second),
	})
	store.PutPodSession(&domain.PodSession{
		ID:        "active",
		Namespace: "default",
		PVCUID:    "active",
		ExpiresAt: now.Add(time.Hour),
	})

	sessions := store.ListExpiredPodSessions(now)
	if len(sessions) != 1 || sessions[0].ID != "expired" {
		t.Fatalf("expired pod sessions = %#v", sessions)
	}
	sessions[0].ID = "mutated"
	got := store.ListExpiredPodSessions(now)
	if len(got) != 1 || got[0].ID != "expired" {
		t.Fatalf("store returned mutable expired pointer: %#v", got)
	}
}

func TestGetPodSessionIncludingExpired(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := New(config.Default().Cache)
	store.PutPodSession(&domain.PodSession{
		ID:        "expired",
		Namespace: "default",
		PVCUID:    "uid",
		ExpiresAt: now.Add(-time.Second),
	})

	got, ok := store.GetPodSessionIncludingExpired("expired")
	if !ok || got.ID != "expired" {
		t.Fatalf("GetPodSessionIncludingExpired() = %#v ok=%v", got, ok)
	}
	got.ID = "mutated"
	again, ok := store.GetPodSessionIncludingExpired("expired")
	if !ok || again.ID != "expired" {
		t.Fatalf("store returned mutable expired pointer: %#v ok=%v", again, ok)
	}
	if _, ok := store.GetPodSession("expired", now); ok {
		t.Fatal("GetPodSession() returned expired session")
	}
}

func TestConsumeAuthRequestIsAtomic(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := New(config.Default().Cache)
	store.CreateAuthRequest(&domain.AuthRequest{
		ID:              "ar_1",
		ViewerSessionID: "vs_1",
		PodSessionID:    "ps_1",
		ExpiresAt:       now.Add(time.Minute),
		CreatedAt:       now,
	}, "hash")

	var wg sync.WaitGroup
	successes := make(chan struct{}, 10)
	for range 10 {
		wg.Go(func() {
			if _, ok := store.ConsumeAuthRequest("ar_1", "hash", now); ok {
				successes <- struct{}{}
			}
		})
	}
	wg.Wait()
	close(successes)

	count := 0
	for range successes {
		count++
	}
	if count != 1 {
		t.Fatalf("successful consumes = %d, want 1", count)
	}
}

func TestConsumeAuthRequestRejectsBadHashAndExpired(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := New(config.Default().Cache)
	store.CreateAuthRequest(&domain.AuthRequest{
		ID:        "bad_hash",
		ExpiresAt: now.Add(time.Minute),
		CreatedAt: now,
	}, "hash")
	if _, ok := store.ConsumeAuthRequest("bad_hash", "wrong", now); ok {
		t.Fatal("bad hash was accepted")
	}

	store.CreateAuthRequest(&domain.AuthRequest{
		ID:        "expired",
		ExpiresAt: now.Add(-time.Second),
		CreatedAt: now.Add(-time.Minute),
	}, "hash")
	if _, ok := store.ConsumeAuthRequest("expired", "hash", now); ok {
		t.Fatal("expired auth request was accepted")
	}
}
