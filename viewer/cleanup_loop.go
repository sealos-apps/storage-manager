package viewer

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type cleanupRunFunc func(context.Context) error
type cleanupErrorFunc func(error)

func startCleanupLoop(
	ctx context.Context,
	interval time.Duration,
	run cleanupRunFunc,
	onError cleanupErrorFunc,
) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	if interval <= 0 || run == nil {
		return cancel
	}
	ticker := time.NewTicker(interval)
	done := runCleanupLoop(ctx, ticker.C, run, onError)
	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			cancel()
			ticker.Stop()
			<-done
		})
	}
}

func runCleanupLoop(
	ctx context.Context,
	ticks <-chan time.Time,
	run cleanupRunFunc,
	onError cleanupErrorFunc,
) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if err := run(ctx); err != nil && onError != nil {
				onError(err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticks:
			}
		}
	}()
	return done
}

func logCleanupError(err error) {
	slog.Error("viewer cleanup failed", "error", err)
}
