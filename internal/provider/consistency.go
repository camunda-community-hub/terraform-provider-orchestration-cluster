package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
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
func waitForConsistency[T any](ctx context.Context, subject string, refresh func() (T, bool, error)) (T, error) {
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
		Timeout:    consistencyPolling.Timeout,
	}

	if _, err := state.WaitForStateContext(ctx); err != nil {
		return last, fmt.Errorf("waiting for %s to become consistent: %w", subject, err)
	}

	return last, nil
}
