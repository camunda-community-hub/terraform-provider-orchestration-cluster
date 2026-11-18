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

	// retryMaxElapsed bounds the total time spent retrying one request.
	retryMaxElapsed = 30 * time.Second
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

	// sleep and jitter are replaced in tests.
	sleep  func(ctx context.Context, d time.Duration) error
	jitter func(d time.Duration) time.Duration
}

func newRetryingHTTPClient(inner camunda.HttpRequestDoer, maxRetries int) camunda.HttpRequestDoer {
	if maxRetries <= 0 {
		return inner
	}
	return &retryingHTTPClient{inner: inner, maxRetries: maxRetries, sleep: sleepContext, jitter: fullJitter}
}

func (c *retryingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	start := time.Now()

	for attempt := 1; ; attempt++ {
		resp, err := c.inner.Do(req)
		if err != nil || !retryable(req, resp.StatusCode) {
			return resp, err
		}

		delay := c.backoff(attempt, resp)
		if attempt > c.maxRetries || time.Since(start)+delay > retryMaxElapsed || !rewindBody(req) {
			resp.Body.Close()
			return nil, fmt.Errorf("%s %s failed with status %d after %d attempt(s)", req.Method, redactURL(req.URL), resp.StatusCode, attempt)
		}

		tflog.Debug(ctx, "retrying HTTP request to orchestration cluster API after 5xx response", map[string]interface{}{
			"http_method": req.Method,
			"http_url":    redactURL(req.URL),
			"http_status": resp.Status,
			"attempt":     attempt,
			"delay":       delay.String(),
		})
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if err := c.sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
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

// rewindBody resets req.Body for another attempt, reporting false when it cannot be replayed.
func rewindBody(req *http.Request) bool {
	if req.Body == nil || req.Body == http.NoBody {
		return true
	}
	if req.GetBody == nil {
		return false
	}
	body, err := req.GetBody()
	if err != nil {
		return false
	}
	req.Body = body
	return true
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
