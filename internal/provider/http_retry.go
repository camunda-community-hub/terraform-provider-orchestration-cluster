package provider

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.10"
)

const (
	// defaultMaxRetries is the number of retries after the first attempt when max_retries is unset.
	defaultMaxRetries = 3

	// maxMaxRetries is the largest accepted max_retries.
	maxMaxRetries = 10

	// retryBaseDelay is the backoff before the first retry. It doubles with every further retry.
	retryBaseDelay = 500 * time.Millisecond

	// retryMaxDelay caps a single backoff, including a server supplied Retry-After.
	retryMaxDelay = 10 * time.Second

	// retryMaxElapsed bounds the time from the first response to the end of the last retry,
	// covering backoff waits, retry attempts and the draining of discarded responses.
	retryMaxElapsed = 30 * time.Second

	// maxDrainBytes is how much of a discarded 5xx response body is read to let its connection be reused.
	maxDrainBytes = 4 << 10
)

// retryingHTTPClient wraps an HttpRequestDoer and repeats a request that got a 5xx response, with
// exponential backoff and jitter. A server supplied Retry-After is honored. Once the retries are
// exhausted it returns an error naming the last status code and the number of attempts.
//
// Idempotent methods (GET, HEAD, PUT, DELETE) are retried on any 5xx, and so are the read-only
// POST /search endpoints. Any other POST is retried only on 503: 502 and 504 can be returned after
// the API committed the operation, and the API has no idempotency key to make a replay safe.
type retryingHTTPClient struct {
	inner      camunda.HttpRequestDoer
	maxRetries int

	// maxElapsed is retryMaxElapsed, replaced in tests.
	maxElapsed time.Duration

	// sleep and jitter are replaced in tests.
	sleep  func(ctx context.Context, d time.Duration) error
	jitter func(d time.Duration) time.Duration
}

func newRetryingHTTPClient(inner camunda.HttpRequestDoer, maxRetries int) camunda.HttpRequestDoer {
	if maxRetries <= 0 {
		return inner
	}
	return &retryingHTTPClient{inner: inner, maxRetries: maxRetries, maxElapsed: retryMaxElapsed, sleep: sleepContext, jitter: fullJitter}
}

func (c *retryingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	var deadline time.Time

	cur := req
	var cancel context.CancelFunc = func() {}
	disarm := func() bool { return true }
	for attempt := 1; ; attempt++ {
		resp, err := c.inner.Do(cur)
		if err != nil {
			cancel()
			if attempt > 1 && ctx.Err() == nil {
				return nil, fmt.Errorf("%s %s did not complete within %s of retrying: %w", req.Method, redactURL(req.URL), c.maxElapsed, err)
			}
			return nil, err
		}
		if attempt == 1 {
			deadline = time.Now().Add(c.maxElapsed)
		}
		if !retryable(req, resp.StatusCode) {
			disarm()
			resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
			return resp, nil
		}

		delay := c.backoff(attempt, resp)
		body, replayable := replayBody(req)
		if attempt > c.maxRetries || time.Now().Add(delay).After(deadline) || !replayable {
			drainBody(resp, deadline)
			cancel()
			return nil, fmt.Errorf("%s %s failed with status %d after %d attempt(s)", req.Method, redactURL(req.URL), resp.StatusCode, attempt)
		}

		tflog.Debug(ctx, "retrying HTTP request to orchestration cluster API after 5xx response", map[string]interface{}{
			"http_method": req.Method,
			"http_url":    redactURL(req.URL),
			"http_status": resp.Status,
			"attempt":     attempt,
			"delay":       delay.String(),
		})
		drainBody(resp, deadline)
		cancel()

		if err := c.sleep(ctx, delay); err != nil {
			return nil, err
		}

		// Retries run under a context that a timer cancels at the shared deadline, so a stalled
		// attempt cannot outlive it. The timer is disarmed once a response is accepted, so the
		// deadline never truncates its body; the context itself is released when that body is
		// closed (see cancelOnClose). The first attempt is not bounded this way: a slow success
		// is not an error.
		var attemptCtx context.Context
		attemptCtx, cancel = context.WithCancel(ctx)
		timer := time.AfterFunc(time.Until(deadline), cancel)
		disarm = timer.Stop
		cur = req.Clone(attemptCtx)
		cur.Body = body
	}
}

// drainBody discards what is left of a response we are about to retry or give up on so the
// connection can be reused, but never reads past deadline or more than a few KiB.
func drainBody(resp *http.Response, deadline time.Time) {
	timer := time.AfterFunc(time.Until(deadline), func() { resp.Body.Close() })
	defer timer.Stop()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))
	resp.Body.Close()
}

// cancelOnClose releases the context of a retry attempt once its response body is closed, so the
// context is never leaked.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// backoff returns the wait before the retry that follows the given attempt.
func (c *retryingHTTPClient) backoff(attempt int, resp *http.Response) time.Duration {
	if d, ok := retryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
		return min(d, retryMaxDelay)
	}
	d := retryBaseDelay << min(attempt-1, 16)
	return c.jitter(min(d, retryMaxDelay))
}

// retryable reports whether a response with status to req is safe to repeat.
func retryable(req *http.Request, status int) bool {
	if status < 500 || status > 599 {
		return false
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
		return true
	case http.MethodPost:
		return status == http.StatusServiceUnavailable || strings.HasSuffix(strings.TrimRight(req.URL.Path, "/"), "/search")
	}
	return false
}

// replayBody returns a fresh copy of req's body for another attempt, reporting false when the
// body cannot be replayed.
func replayBody(req *http.Request) (io.ReadCloser, bool) {
	if req.Body == nil || req.Body == http.NoBody {
		return req.Body, true
	}
	if req.GetBody == nil {
		return nil, false
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, false
	}
	return body, true
}

// retryAfter parses a Retry-After header, either delay-seconds or an HTTP date.
func retryAfter(value string, now time.Time) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(value); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(value); err == nil {
		return max(t.Sub(now), 0), true
	}
	return 0, false
}

// fullJitter returns a random duration in [d/2, d].
func fullJitter(d time.Duration) time.Duration {
	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
