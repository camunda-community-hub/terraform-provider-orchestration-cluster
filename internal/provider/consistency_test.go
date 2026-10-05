package provider

import (
	"context"
	"errors"
	"fmt"
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

func idOfString(s string) string { return s }

// sequenceSearch returns a search func that replays the given result sets in order,
// repeating the last one once exhausted.
func sequenceSearch(calls *int, sets ...[]string) func() ([]string, error) {
	return func() ([]string, error) {
		i := *calls
		*calls++
		if i >= len(sets) {
			i = len(sets) - 1
		}
		return sets[i], nil
	}
}

func TestSearchByNameUntilStable_StabilizesOnSameIDs(t *testing.T) {
	withFastConsistencyPolling(t)

	calls := 0
	items, matchCount, hardErr, err := searchByNameUntilStable(context.Background(), "widget", sequenceSearch(&calls, []string{"a"}), idOfString)

	if err != nil || hardErr != nil {
		t.Fatalf("unexpected errors: err=%v hardErr=%v", err, hardErr)
	}
	if len(items) != 1 || items[0] != "a" {
		t.Fatalf("got items %v, want [a]", items)
	}
	if matchCount != 1 {
		t.Fatalf("got matchCount %d, want 1", matchCount)
	}
	if calls != 2 {
		t.Fatalf("expected 2 polls to confirm stability, got %d", calls)
	}
}

func TestSearchByNameUntilStable_ReorderedResultsAreStable(t *testing.T) {
	withFastConsistencyPolling(t)

	calls := 0
	_, matchCount, _, err := searchByNameUntilStable(context.Background(), "widget",
		sequenceSearch(&calls, []string{"a", "b"}, []string{"b", "a"}), idOfString)

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if matchCount != 2 {
		t.Fatalf("got matchCount %d, want 2", matchCount)
	}
	if calls != 2 {
		t.Fatalf("expected reordering to count as stable after 2 polls, got %d", calls)
	}
}

func TestSearchByNameUntilStable_SameCountDifferentIDsIsNotStable(t *testing.T) {
	withFastConsistencyPolling(t)

	calls := 0
	items, matchCount, _, err := searchByNameUntilStable(context.Background(), "widget",
		sequenceSearch(&calls, []string{"a"}, []string{"b"}, []string{"b"}), idOfString)

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(items) != 1 || items[0] != "b" {
		t.Fatalf("got items %v, want [b]", items)
	}
	if matchCount != 1 {
		t.Fatalf("got matchCount %d, want 1", matchCount)
	}
	if calls != 3 {
		t.Fatalf("expected a/b swap to delay acceptance until the third poll, got %d polls", calls)
	}
}

func TestSearchByNameUntilStable_ZeroResultsTimesOutAsNotFound(t *testing.T) {
	withFastConsistencyPolling(t)
	consistencyPolling.Timeout = 300 * time.Millisecond

	calls := 0
	_, matchCount, hardErr, err := searchByNameUntilStable(context.Background(), "widget", sequenceSearch(&calls, []string{}), idOfString)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if hardErr != nil {
		t.Fatalf("unexpected hardErr: %v", hardErr)
	}
	if matchCount != 0 {
		t.Fatalf("got matchCount %d, want 0 (not found)", matchCount)
	}
}

func TestSearchByNameUntilStable_UnstablePositiveCountTimesOut(t *testing.T) {
	withFastConsistencyPolling(t)
	consistencyPolling.Timeout = 300 * time.Millisecond

	n := 0
	search := func() ([]string, error) {
		n++
		return []string{fmt.Sprintf("id-%d", n)}, nil
	}
	_, matchCount, hardErr, err := searchByNameUntilStable(context.Background(), "widget", search, idOfString)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if hardErr != nil {
		t.Fatalf("unexpected hardErr: %v", hardErr)
	}
	if matchCount != 1 {
		t.Fatalf("got matchCount %d, want 1 (unstable, not not-found)", matchCount)
	}
}

func TestSearchByNameUntilStable_HardErrorIsReported(t *testing.T) {
	withFastConsistencyPolling(t)

	wantErr := errors.New("boom")
	calls := 0
	_, matchCount, hardErr, err := searchByNameUntilStable(context.Background(), "widget",
		func() ([]string, error) {
			calls++
			return nil, wantErr
		}, idOfString)

	if !errors.Is(hardErr, wantErr) {
		t.Fatalf("got hardErr %v, want %v", hardErr, wantErr)
	}
	if err == nil {
		t.Fatal("expected a wrapped error, got nil")
	}
	if matchCount != -1 {
		t.Fatalf("got matchCount %d, want -1 (no completed poll)", matchCount)
	}
	if calls != 1 {
		t.Fatalf("expected hard error to stop polling immediately, got %d calls", calls)
	}
}

func TestSearchByNameUntilStable_CancellationBeforeFirstPoll(t *testing.T) {
	withFastConsistencyPolling(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	_, matchCount, hardErr, err := searchByNameUntilStable(ctx, "widget", sequenceSearch(&calls, []string{"a"}), idOfString)

	if err == nil {
		t.Fatal("expected a cancellation error, got nil")
	}
	if hardErr != nil {
		t.Fatalf("unexpected hardErr: %v", hardErr)
	}
	if matchCount != -1 {
		t.Fatalf("got matchCount %d, want -1 (cancelled before first poll)", matchCount)
	}
}
