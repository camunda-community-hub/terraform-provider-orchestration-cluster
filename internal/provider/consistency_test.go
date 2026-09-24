package provider

import (
	"context"
	"errors"
	"testing"
	"time"
)

// withFastConsistencyPolling temporarily shrinks the package-level polling
// timing so these unit tests don't have to wait out the real (multi-second)
// production defaults, and restores it afterward.
func withFastConsistencyPolling(t *testing.T) {
	t.Helper()

	original := consistencyPolling
	consistencyPolling.Delay = 10 * time.Millisecond
	consistencyPolling.MinTimeout = 50 * time.Millisecond
	consistencyPolling.Timeout = 2 * time.Second

	t.Cleanup(func() {
		consistencyPolling = original
	})
}

func TestWaitForConsistency_ImmediateSuccess(t *testing.T) {
	calls := 0
	got, err := waitForConsistency(context.Background(), "widget \"a\"", func() (string, bool, error) {
		calls++
		return "value", true, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got != "value" {
		t.Fatalf("got %q, want %q", got, "value")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 refresh call, got %d", calls)
	}
}

func TestWaitForConsistency_EventuallyConsistent(t *testing.T) {
	withFastConsistencyPolling(t)

	calls := 0
	got, err := waitForConsistency(context.Background(), "widget \"a\"", func() (int, bool, error) {
		calls++
		// Not consistent yet for the first two reads, consistent on the third.
		return calls, calls >= 3, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
	if calls < 3 {
		t.Fatalf("expected at least 3 refresh calls, got %d", calls)
	}
}

func TestWaitForConsistency_HardErrorStopsImmediately(t *testing.T) {
	withFastConsistencyPolling(t)

	wantErr := errors.New("boom")
	calls := 0
	_, err := waitForConsistency(context.Background(), "widget \"a\"", func() (string, bool, error) {
		calls++
		return "", false, wantErr
	})

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error to wrap %v, got %v", wantErr, err)
	}
	if calls != 1 {
		t.Fatalf("expected the hard error to stop polling immediately (1 call), got %d", calls)
	}
}

func TestWaitForConsistency_NeverConsistentTimesOut(t *testing.T) {
	withFastConsistencyPolling(t)

	_, err := waitForConsistency(context.Background(), "widget \"a\"", func() (string, bool, error) {
		return "stale", false, nil
	})

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
}
