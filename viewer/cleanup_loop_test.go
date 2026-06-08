package viewer

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCleanupLoopRunsImmediatelyAndOnTicks(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ticks := make(chan time.Time)
	calls := make(chan struct{}, 2)

	done := runCleanupLoop(ctx, ticks, func(context.Context) error {
		calls <- struct{}{}
		return nil
	}, nil)

	waitForCall(t, calls)
	ticks <- time.Now()
	waitForCall(t, calls)

	cancel()
	waitForLoopDone(t, done)
}

func TestCleanupLoopContinuesAfterError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ticks := make(chan time.Time)
	calls := make(chan struct{}, 2)
	logged := make(chan error, 1)
	failure := errors.New("cleanup failed")
	callCount := 0

	done := runCleanupLoop(ctx, ticks, func(context.Context) error {
		callCount++
		calls <- struct{}{}
		if callCount == 1 {
			return failure
		}
		return nil
	}, func(err error) {
		logged <- err
	})

	waitForCall(t, calls)
	if got := waitForError(t, logged); !errors.Is(got, failure) {
		t.Fatalf("logged error = %v, want %v", got, failure)
	}
	ticks <- time.Now()
	waitForCall(t, calls)

	cancel()
	waitForLoopDone(t, done)
}

func TestCleanupLoopStopsOnCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	calls := make(chan struct{}, 1)

	done := runCleanupLoop(ctx, ticks, func(context.Context) error {
		calls <- struct{}{}
		return nil
	}, nil)
	waitForCall(t, calls)
	cancel()
	waitForLoopDone(t, done)

	select {
	case ticks <- time.Now():
		t.Fatal("cleanup loop received tick after cancellation")
	default:
	}
}

func TestStartCleanupLoopIgnoresNonPositiveInterval(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := make(chan struct{}, 1)

	stop := startCleanupLoop(ctx, 0, func(context.Context) error {
		calls <- struct{}{}
		return nil
	}, nil)
	defer stop()

	select {
	case <-calls:
		t.Fatal("cleanup ran with non-positive interval")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestStartCleanupLoopStopIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stop := startCleanupLoop(ctx, time.Hour, func(context.Context) error {
		return nil
	}, nil)

	stop()
	stop()
}

func waitForCall(t *testing.T, calls <-chan struct{}) {
	t.Helper()

	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cleanup call")
	}
}

func waitForError(t *testing.T, errs <-chan error) error {
	t.Helper()

	select {
	case err := <-errs:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cleanup error")
		return nil
	}
}

func waitForLoopDone(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cleanup loop to stop")
	}
}
