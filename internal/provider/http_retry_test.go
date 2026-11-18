package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// flakyServer answers failures times with status, then 200. It counts requests and records bodies.
func flakyServer(t *testing.T, failures int, status int, header http.Header) (*httptest.Server, *atomic.Int32, *[]string) {
	t.Helper()
	var calls atomic.Int32
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if int(calls.Add(1)) <= failures {
			for k, v := range header {
				w.Header()[k] = v
			}
			w.WriteHeader(status)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &bodies
}

func testRetryClient(maxRetries int, slept *[]time.Duration) *retryingHTTPClient {
	return &retryingHTTPClient{
		inner:      http.DefaultClient,
		maxRetries: maxRetries,
		sleep: func(_ context.Context, d time.Duration) error {
			*slept = append(*slept, d)
			return nil
		},
		jitter: func(d time.Duration) time.Duration { return d },
	}
}

func doRequest(t *testing.T, c *retryingHTTPClient, method, url, body string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return c.Do(req)
}

func TestRetryingHTTPClient_RecoversFrom5xx(t *testing.T) {
	srv, calls, _ := flakyServer(t, 2, http.StatusServiceUnavailable, nil)
	var slept []time.Duration

	resp, err := doRequest(t, testRetryClient(3, &slept), http.MethodGet, srv.URL, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK || calls.Load() != 3 {
		t.Fatalf("status %d after %d calls, want 200 after 3", resp.StatusCode, calls.Load())
	}
	if len(slept) != 2 || slept[1] != 2*slept[0] {
		t.Fatalf("backoff %v, want two waits with the second double the first", slept)
	}
}

func TestRetryingHTTPClient_ExhaustedNamesStatusAndAttempts(t *testing.T) {
	srv, calls, _ := flakyServer(t, 100, http.StatusBadGateway, nil)
	var slept []time.Duration

	_, err := doRequest(t, testRetryClient(2, &slept), http.MethodDelete, srv.URL, "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "status 502") || !strings.Contains(err.Error(), "after 3 attempt(s)") {
		t.Fatalf("error %q does not name the status code and attempt count", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("%d calls, want 3", calls.Load())
	}
}

func TestRetryingHTTPClient_PostOnlyRetriedOnGatewayErrors(t *testing.T) {
	var slept []time.Duration

	srv, calls, bodies := flakyServer(t, 1, http.StatusInternalServerError, nil)
	resp, err := doRequest(t, testRetryClient(3, &slept), http.MethodPost, srv.URL, "payload")
	if err != nil || resp.StatusCode != http.StatusInternalServerError || calls.Load() != 1 {
		t.Fatalf("POST 500: resp %v err %v calls %d, want the 500 returned after 1 call", resp, err, calls.Load())
	}
	_ = bodies

	srv, calls, bodies = flakyServer(t, 1, http.StatusServiceUnavailable, nil)
	resp, err = doRequest(t, testRetryClient(3, &slept), http.MethodPost, srv.URL, "payload")
	if err != nil || resp.StatusCode != http.StatusOK || calls.Load() != 2 {
		t.Fatalf("POST 503: resp %v err %v calls %d, want 200 after 2 calls", resp, err, calls.Load())
	}
	if len(*bodies) != 2 || (*bodies)[0] != "payload" || (*bodies)[1] != "payload" {
		t.Fatalf("bodies %q, want the body replayed on retry", *bodies)
	}
}

func TestRetryingHTTPClient_DoesNotRetryClientErrors(t *testing.T) {
	srv, calls, _ := flakyServer(t, 5, http.StatusNotFound, nil)
	var slept []time.Duration

	resp, err := doRequest(t, testRetryClient(3, &slept), http.MethodGet, srv.URL, "")
	if err != nil || resp.StatusCode != http.StatusNotFound || calls.Load() != 1 {
		t.Fatalf("resp %v err %v calls %d, want the 404 returned after 1 call", resp, err, calls.Load())
	}
}

func TestRetryingHTTPClient_HonorsRetryAfter(t *testing.T) {
	srv, _, _ := flakyServer(t, 1, http.StatusServiceUnavailable, http.Header{"Retry-After": {"3"}})
	var slept []time.Duration

	if _, err := doRequest(t, testRetryClient(3, &slept), http.MethodGet, srv.URL, ""); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 1 || slept[0] != 3*time.Second {
		t.Fatalf("waits %v, want [3s]", slept)
	}
}

func TestRetryingHTTPClient_StopsWhenContextCancelled(t *testing.T) {
	srv, calls, _ := flakyServer(t, 100, http.StatusServiceUnavailable, nil)
	ctx, cancel := context.WithCancel(context.Background())
	c := &retryingHTTPClient{
		inner:      http.DefaultClient,
		maxRetries: 5,
		sleep:      func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() },
		jitter:     func(d time.Duration) time.Duration { return d },
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	if _, err := c.Do(req); err == nil || calls.Load() != 1 {
		t.Fatalf("err %v calls %d, want a context error after 1 call", err, calls.Load())
	}
}

func TestNewRetryingHTTPClient_ZeroDisables(t *testing.T) {
	if got := newRetryingHTTPClient(http.DefaultClient, 0); got != http.DefaultClient {
		t.Fatalf("got %T, want the inner client unchanged", got)
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	for value, want := range map[string]time.Duration{
		"5": 5 * time.Second,
		now.Add(7 * time.Second).Format(http.TimeFormat): 7 * time.Second,
		now.Add(-time.Minute).Format(http.TimeFormat):    0,
	} {
		if got, ok := retryAfter(value, now); !ok || got != want {
			t.Errorf("retryAfter(%q) = %v, %v; want %v", value, got, ok, want)
		}
	}
	for _, value := range []string{"", "-1", "soon"} {
		if _, ok := retryAfter(value, now); ok {
			t.Errorf("retryAfter(%q) accepted, want rejected", value)
		}
	}
}
