package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-log/tflogtest"
)

// partialErrorReader yields data once, then returns err on every subsequent
// Read, simulating a response body stream that fails partway through.
type partialErrorReader struct {
	data []byte
	err  error
	pos  int
}

func (r *partialErrorReader) Read(p []byte) (int, error) {
	if r.pos < len(r.data) {
		n := copy(p, r.data[r.pos:])
		r.pos += n
		return n, nil
	}
	return 0, r.err
}

func (r *partialErrorReader) Close() error { return nil }

type fakeDoer struct {
	resp        *http.Response
	err         error
	gotBody     []byte
	gotHeader   http.Header
	requestSeen *http.Request
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.requestSeen = req
	f.gotHeader = req.Header
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		f.gotBody = body
	}
	return f.resp, f.err
}

// TestLoggingHTTPClient_PassesRequestBodyThrough verifies that logging the
// request body doesn't consume it before the underlying Doer sees it.
func TestLoggingHTTPClient_PassesRequestBodyThrough(t *testing.T) {
	t.Setenv("TF_LOG", "DEBUG")
	t.Setenv("TF_LOG_PROVIDER", "")

	inner := &fakeDoer{
		resp: &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok":true}`))),
		},
	}
	client := newLoggingHTTPClient(inner)

	req, err := http.NewRequest(http.MethodPost, "https://example.invalid/foo", bytes.NewReader([]byte(`{"name":"widget"}`)))
	if err != nil {
		t.Fatalf("unexpected error building request: %s", err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if string(inner.gotBody) != `{"name":"widget"}` {
		t.Fatalf("inner doer got body %q, want %q", inner.gotBody, `{"name":"widget"}`)
	}
	if got := inner.gotHeader.Get("Authorization"); got != "Bearer secret-token" {
		t.Fatalf("inner doer got Authorization %q, want unredacted value to still be sent", got)
	}

	// The response body must still be readable by the caller after the
	// logging wrapper has read it for logging purposes.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("unexpected error reading response body: %s", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("got response body %q, want %q", body, `{"ok":true}`)
	}
}

// TestLoggingHTTPClient_RedactsCredentialsInEmittedLogs verifies the actual
// tflog.Debug output for a credential-bearing request and response: none of
// the sensitive header or body values may appear anywhere in the emitted log
// entries, while ordinary data must still come through.
func TestLoggingHTTPClient_RedactsCredentialsInEmittedLogs(t *testing.T) {
	t.Setenv("TF_LOG", "DEBUG")
	t.Setenv("TF_LOG_PROVIDER", "")

	inner := &fakeDoer{
		resp: &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Header:     http.Header{"Set-Cookie": []string{"session=resp-secret-cookie"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"username":"alice","password":"resp-secret-password"}`))),
		},
	}
	client := newLoggingHTTPClient(inner)

	var logs bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &logs)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.invalid/foo",
		bytes.NewReader([]byte(`{"username":"alice","password":"req-secret-password"}`)))
	if err != nil {
		t.Fatalf("unexpected error building request: %s", err)
	}
	req.Header.Set("Authorization", "Bearer req-secret-token")
	req.Header.Set("Cookie", "session=req-secret-cookie")

	if _, err := client.Do(req); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	entries, err := tflogtest.MultilineJSONDecode(&logs)
	if err != nil {
		t.Fatalf("unexpected error decoding log output: %s", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d log entries, want 2 (request + response)", len(entries))
	}

	secrets := []string{
		"req-secret-token", "req-secret-cookie", "req-secret-password",
		"resp-secret-cookie", "resp-secret-password",
	}
	for _, entry := range entries {
		dump := fmt.Sprintf("%v", entry)
		for _, secret := range secrets {
			if strings.Contains(dump, secret) {
				t.Fatalf("log entry leaked credential %q: %v", secret, entry)
			}
		}
		if !strings.Contains(dump, "alice") {
			t.Fatalf("log entry dropped ordinary data alongside redaction: %v", entry)
		}
	}
}

// TestLoggingHTTPClient_SkipsBufferingWhenDebugDisabled verifies that when
// debug logging isn't enabled, the wrapper is a pure passthrough: it doesn't
// touch the request body at all.
func TestLoggingHTTPClient_SkipsBufferingWhenDebugDisabled(t *testing.T) {
	t.Setenv("TF_LOG", "")
	t.Setenv("TF_LOG_PROVIDER", "")

	inner := &fakeDoer{
		resp: &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok":true}`))),
		},
	}
	client := newLoggingHTTPClient(inner)

	req, err := http.NewRequest(http.MethodPost, "https://example.invalid/foo", bytes.NewReader([]byte(`{"name":"widget"}`)))
	if err != nil {
		t.Fatalf("unexpected error building request: %s", err)
	}

	if _, err := client.Do(req); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if string(inner.gotBody) != `{"name":"widget"}` {
		t.Fatalf("inner doer got body %q, want %q", inner.gotBody, `{"name":"widget"}`)
	}
}

// TestLoggingHTTPClient_PreservesResponseOnBodyReadError verifies that a
// response body read failure under debug logging still surfaces the caller's
// response (status/headers) and the partial body, with the read error only
// encountered when the caller reads resp.Body — matching the semantics of an
// unwrapped http.Response.Body.
func TestLoggingHTTPClient_PreservesResponseOnBodyReadError(t *testing.T) {
	t.Setenv("TF_LOG", "DEBUG")
	t.Setenv("TF_LOG_PROVIDER", "")

	wantErr := errors.New("connection reset by peer")
	inner := &fakeDoer{
		resp: &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       &partialErrorReader{data: []byte(`{"partial":`), err: wantErr},
		},
	}
	client := newLoggingHTTPClient(inner)

	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/foo", nil)
	if err != nil {
		t.Fatalf("unexpected error building request: %s", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do returned an error %q, want the read error to surface from resp.Body instead", err)
	}
	if resp == nil {
		t.Fatal("Do returned a nil response, want the response with its status/headers preserved")
	}
	if resp.Status != "200 OK" {
		t.Fatalf("resp.Status = %q, want %q", resp.Status, "200 OK")
	}

	body, readErr := io.ReadAll(resp.Body)
	if string(body) != `{"partial":` {
		t.Fatalf("got partial body %q, want %q", body, `{"partial":`)
	}
	if !errors.Is(readErr, wantErr) {
		t.Fatalf("got read error %v, want %v", readErr, wantErr)
	}
}

// TestLoggingHTTPClient_LogsFinalURLAfterRedirect verifies that when the
// inner Doer follows a redirect (as http.Client.Do does internally), the
// response is logged with the final URL that actually produced it, not the
// original pre-redirect request URL.
func TestLoggingHTTPClient_LogsFinalURLAfterRedirect(t *testing.T) {
	t.Setenv("TF_LOG", "DEBUG")
	t.Setenv("TF_LOG_PROVIDER", "")

	finalURL, err := url.Parse("https://example.invalid/bar")
	if err != nil {
		t.Fatalf("unexpected error parsing URL: %s", err)
	}

	inner := &fakeDoer{
		resp: &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    &http.Request{URL: finalURL},
		},
	}
	client := newLoggingHTTPClient(inner)

	var logs bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &logs)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid/foo", nil)
	if err != nil {
		t.Fatalf("unexpected error building request: %s", err)
	}

	if _, err := client.Do(req); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	entries, err := tflogtest.MultilineJSONDecode(&logs)
	if err != nil {
		t.Fatalf("unexpected error decoding log output: %s", err)
	}

	var gotURL string
	var found bool
	for _, entry := range entries {
		if entry["@message"] == "received HTTP response from orchestration cluster API" {
			gotURL, _ = entry["http_url"].(string)
			found = true
		}
	}
	if !found {
		t.Fatal("did not find the expected response log entry")
	}
	if want := "https://example.invalid/bar"; gotURL != want {
		t.Fatalf("logged http_url = %q, want the final (post-redirect) URL %q", gotURL, want)
	}
}

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "strips embedded password",
			url:  "https://user:s3cret@example.invalid/foo",
			want: "https://user:xxxxx@example.invalid/foo",
		},
		{
			name: "leaves URLs without userinfo unchanged",
			url:  "https://example.invalid/foo?bar=baz",
			want: "https://example.invalid/foo?bar=baz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			if err != nil {
				t.Fatalf("unexpected error parsing URL: %s", err)
			}
			if got := redactURL(u); got != tt.want {
				t.Fatalf("redactURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestRedactHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization": []string{"Bearer secret-token"},
		"Cookie":        []string{"session=abc123"},
		"Content-Type":  []string{"application/json"},
	}

	redacted := redactHeaders(headers)

	if redacted["Authorization"] != "REDACTED" {
		t.Fatalf("Authorization header not redacted: %q", redacted["Authorization"])
	}
	if redacted["Cookie"] != "REDACTED" {
		t.Fatalf("Cookie header not redacted: %q", redacted["Cookie"])
	}
	if redacted["Content-Type"] != "application/json" {
		t.Fatalf("Content-Type header unexpectedly changed: %q", redacted["Content-Type"])
	}
}

func TestRedactBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "redacts top-level password",
			body: `{"username":"alice","password":"s3cret"}`,
			want: `{"password":"REDACTED","username":"alice"}`,
		},
		{
			name: "redacts nested sensitive fields",
			body: `{"user":{"name":"alice","password":"s3cret"},"oidc":{"client_secret":"abc"}}`,
			want: `{"oidc":{"client_secret":"REDACTED"},"user":{"name":"alice","password":"REDACTED"}}`,
		},
		{
			name: "redacts sensitive fields inside arrays",
			body: `[{"password":"s3cret"},{"name":"bob"}]`,
			want: `[{"password":"REDACTED"},{"name":"bob"}]`,
		},
		{
			name: "leaves non-sensitive bodies unchanged",
			body: `{"name":"widget","count":3}`,
			want: `{"count":3,"name":"widget"}`,
		},
		{
			name: "non-JSON body is never logged raw",
			body: `not json`,
			want: `<non-JSON body omitted>`,
		},
		{
			name: "preserves large integers beyond float64 precision",
			body: `{"timestamp":9007199254740993}`,
			want: `{"timestamp":9007199254740993}`,
		},
		{
			name: "rejects trailing JSON data like Unmarshal does",
			body: `{"name":"widget"}{"name":"widget2"}`,
			want: `<non-JSON body omitted>`,
		},
		{
			name: "rejects a stray trailing brace after a valid object",
			body: `{"name":"widget"}}`,
			want: `<non-JSON body omitted>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactBody([]byte(tt.body))
			if got != tt.want {
				t.Fatalf("redactBody(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

func TestDebugLoggingEnabled(t *testing.T) {
	tests := []struct {
		name        string
		tfLog       string
		tfLogSet    bool
		providerLog string
		providerSet bool
		want        bool
	}{
		{name: "nothing set", want: false},
		{name: "TF_LOG=DEBUG", tfLog: "DEBUG", tfLogSet: true, want: true},
		{name: "TF_LOG=trace lowercase", tfLog: "trace", tfLogSet: true, want: true},
		{name: "TF_LOG=WARN", tfLog: "WARN", tfLogSet: true, want: false},
		{name: "TF_LOG_PROVIDER overrides TF_LOG when set", tfLog: "DEBUG", tfLogSet: true, providerLog: "WARN", providerSet: true, want: false},
		{name: "TF_LOG_PROVIDER=JSON enables regardless of TF_LOG", tfLog: "WARN", tfLogSet: true, providerLog: "JSON", providerSet: true, want: true},
		{name: "TF_LOG_PROVIDER empty falls back to TF_LOG", tfLog: "DEBUG", tfLogSet: true, providerLog: "", providerSet: false, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.tfLogSet {
				t.Setenv("TF_LOG", tt.tfLog)
			} else {
				t.Setenv("TF_LOG", "")
			}
			if tt.providerSet {
				t.Setenv("TF_LOG_PROVIDER", tt.providerLog)
			} else {
				t.Setenv("TF_LOG_PROVIDER", "")
			}

			if got := debugLoggingEnabled(); got != tt.want {
				t.Fatalf("debugLoggingEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
