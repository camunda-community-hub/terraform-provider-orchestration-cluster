package provider

import (
	"testing"
	"time"
)

// notFoundConsistencyTimeout is the polling timeout applied around data source steps that
// expect a not-found error. In production a lookup that finds nothing keeps polling for the
// full consistencyPolling.Timeout, because the read side may simply not have caught up with a
// create yet. Those acceptance steps already know the object does not exist, so waiting the
// whole default only slows the suite.
const notFoundConsistencyTimeout = 5 * time.Second

// consistencyPollingDefault captures the production polling configuration so a step can
// restore it after shortening the timeout.
var consistencyPollingDefault = consistencyPolling

// shortConsistencyTimeout returns a TestStep.PreConfig hook that shortens the consistency
// timeout for the steps that follow it, until restoreConsistencyTimeout runs. The provider
// runs in-process in acceptance tests, so it shares consistencyPolling with the test code.
// The configuration is always restored when the test ends.
func shortConsistencyTimeout(t *testing.T) func() {
	t.Helper()
	t.Cleanup(restoreConsistencyTimeout)

	return func() {
		consistencyPolling.Timeout = notFoundConsistencyTimeout
	}
}

// restoreConsistencyTimeout puts the production polling configuration back. Use it as the
// PreConfig of the first step after a not-found step that needs the real timeout again.
func restoreConsistencyTimeout() {
	consistencyPolling = consistencyPollingDefault
}
