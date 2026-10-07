package provider

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
)

// consistencyPolling holds the default timing for polling an eventually-consistent
// read after a write. The orchestration cluster's read-side projection can lag
// briefly behind a create, update, or delete, so a resource must not trust the
// first read that immediately follows a mutation.
var consistencyPolling = struct {
	Delay      time.Duration
	MinTimeout time.Duration
	Timeout    time.Duration
}{
	Delay:      1 * time.Second,
	MinTimeout: 2 * time.Second,
	Timeout:    30 * time.Second,
}

// waitForConsistency polls refresh until it reports the read as consistent with
// the expected post-write state, or until the timeout elapses. It centralizes
// the eventual-consistency handling that resources in this provider would
// otherwise each hand-roll around retry.StateChangeConf.
//
// refresh performs one read and reports:
//   - (value, true, nil)   the read reflects the expected state; stop polling
//   - (value, false, nil)  not yet reflected; keep polling
//   - (zero value, _, err) a hard error; stop polling and return it
//
// subject is used only to produce a readable timeout error, e.g. `tenant "acme"`.
//
// client selects the timeout: it is the consistency_timeout of the provider instance that
// created client (see consistencyTimeoutFor). A nil client uses the default.
func waitForConsistency[T any](ctx context.Context, client *camunda.ClientWithResponses, subject string, refresh func() (T, bool, error)) (T, error) {
	const pending = "pending"
	const ready = "ready"

	var last T
	state := &retry.StateChangeConf{
		Pending: []string{pending},
		Target:  []string{ready},

		// How many times the target state has to be reached to continue.
		ContinuousTargetOccurence: 1,

		Refresh: func() (any, string, error) {
			value, ok, err := refresh()
			if err != nil {
				return nil, "", err
			}

			last = value

			if ok {
				return value, ready, nil
			}
			return value, pending, nil
		},

		Delay:      consistencyPolling.Delay,
		MinTimeout: consistencyPolling.MinTimeout,
		Timeout:    consistencyTimeoutFor(client),
	}

	if _, err := state.WaitForStateContext(ctx); err != nil {
		return last, fmt.Errorf("waiting for %s to become consistent: %w", subject, err)
	}

	return last, nil
}

// readWithRetry wraps waitForConsistency for the common "poll GET until it succeeds"
// shape used right after a create: get performs one read, isReady reports whether that
// read is usable yet.
func readWithRetry[R any](ctx context.Context, client *camunda.ClientWithResponses, label string, get func() (R, error), isReady func(R) (bool, error)) (R, error) {
	return waitForConsistency(ctx, client, label, func() (R, bool, error) {
		value, err := get()
		if err != nil {
			var zero R
			return zero, false, err
		}

		ready, err := isReady(value)
		if err != nil {
			var zero R
			return zero, false, err
		}

		return value, ready, nil
	})
}

// readUntilConsistent wraps waitForConsistency for the common "poll GET until it
// reflects the expected post-write values" shape used right after an update: get
// performs one read, isConsistent reports whether that read already matches what was
// just written.
func readUntilConsistent[R any](ctx context.Context, client *camunda.ClientWithResponses, label string, get func() (R, error), isConsistent func(R) (bool, error)) (R, error) {
	return waitForConsistency(ctx, client, label, func() (R, bool, error) {
		value, err := get()
		if err != nil {
			var zero R
			return zero, false, err
		}

		consistent, err := isConsistent(value)
		if err != nil {
			var zero R
			return zero, false, err
		}

		return value, consistent, nil
	})
}

// searchByNameUntilStable polls search until it returns a nonempty set of items whose
// IDs (as reported by idOf) are unchanged across two consecutive polls, or until the
// timeout elapses. Comparing IDs rather than just the count catches the read-side
// projection returning a different single match on each poll (e.g. one result dropping
// out while another appears), which a count-only comparison would wrongly accept as a
// stable, unique match.
//
// It returns the stabilized items, along with matchCount (the item count from the final
// poll: -1 if no poll ever completed, 0 if the last poll cleanly found no matches, or
// >0 if matches kept appearing but the set never stabilized) and hardErr (the error
// returned directly by search on its final call, as opposed to err, which is
// waitForConsistency's wrapped timeout/cancellation error). Callers use matchCount and
// hardErr to classify the failure: a not-found case (matchCount == 0), an ambiguous or
// unstable case (matchCount > 0), or a hard search error (hardErr != nil).
func searchByNameUntilStable[T any](ctx context.Context, client *camunda.ClientWithResponses, label string, search func() ([]T, error), idOf func(T) string) (items []T, matchCount int, hardErr error, err error) {
	matchCount = -1
	var lastIDs []string

	items, err = waitForConsistency(ctx, client, label, func() ([]T, bool, error) {
		hardErr = nil

		results, searchErr := search()
		if searchErr != nil {
			hardErr = searchErr
			return nil, false, hardErr
		}

		ids := make([]string, len(results))
		for i, item := range results {
			ids[i] = idOf(item)
		}
		sort.Strings(ids)

		stable := len(results) > 0 && len(results) == matchCount && slices.Equal(ids, lastIDs)
		matchCount = len(results)
		lastIDs = ids

		return results, stable, nil
	})

	return items, matchCount, hardErr, err
}
