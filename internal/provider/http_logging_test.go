package provider

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

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
