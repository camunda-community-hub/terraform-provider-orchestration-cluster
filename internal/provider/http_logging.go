package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

// sensitiveHeaders lists request/response headers whose values must never be
// written to logs, since they carry credentials.
var sensitiveHeaders = map[string]bool{
	"authorization": true,
	"cookie":        true,
	"set-cookie":    true,
}

// sensitiveBodyFields lists JSON body field names (matched case-insensitively)
// whose values must be redacted before logging, since they carry credentials
// (e.g. UserRequest.Password in pkg/camunda/8.9/client.gen.go).
var sensitiveBodyFields = map[string]bool{
	"password":      true,
	"secret":        true,
	"client_secret": true,
	"token":         true,
}

// loggingHTTPClient wraps an HttpRequestDoer and logs every request it sends
// and response it receives via tflog.Debug, so that running Terraform with
// TF_LOG=DEBUG surfaces the HTTP traffic between the provider and the
// orchestration cluster API. When debug logging isn't enabled, requests pass
// through untouched: bodies aren't read or re-buffered.
type loggingHTTPClient struct {
	inner camunda.HttpRequestDoer
}

func newLoggingHTTPClient(inner camunda.HttpRequestDoer) camunda.HttpRequestDoer {
	return &loggingHTTPClient{inner: inner}
}

func (c *loggingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if !debugLoggingEnabled() {
		return c.inner.Do(req)
	}

	ctx := req.Context()

	fields := map[string]interface{}{
		"http_method":  req.Method,
		"http_url":     redactURL(req.URL),
		"http_headers": redactHeaders(req.Header),
	}
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		if len(body) > 0 {
			fields["http_request_body"] = redactBody(body)
		}
	}
	tflog.Debug(ctx, "sending HTTP request to orchestration cluster API", fields)

	resp, err := c.inner.Do(req)
	if err != nil {
		tflog.Debug(ctx, "HTTP request to orchestration cluster API failed", map[string]interface{}{
			"http_method": req.Method,
			"http_url":    redactURL(req.URL),
			"error":       err.Error(),
		})
		return resp, err
	}

	respURL := req.URL
	if resp.Request != nil && resp.Request.URL != nil {
		respURL = resp.Request.URL
	}
	respFields := map[string]interface{}{
		"http_method":  req.Method,
		"http_url":     redactURL(respURL),
		"http_status":  resp.Status,
		"http_headers": redactHeaders(resp.Header),
	}
	if resp.Body != nil {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			resp.Body = &errorReplayReadCloser{r: bytes.NewReader(body), err: readErr}
			respFields["error"] = readErr.Error()
			tflog.Debug(ctx, "received HTTP response from orchestration cluster API but failed to read its body", respFields)
			return resp, nil //nolint:nilerr // readErr is deliberately deferred to resp.Body's Read, not returned here
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		if len(body) > 0 {
			respFields["http_response_body"] = redactBody(body)
		}
	}
	tflog.Debug(ctx, "received HTTP response from orchestration cluster API", respFields)

	return resp, nil
}

// errorReplayReadCloser lets a caller read whatever bytes were buffered
// before a response body read failed, then surfaces the original read error
// once those bytes are exhausted. This preserves the semantics callers get
// from an unwrapped http.Response.Body, where a stream error surfaces at
// read time rather than from Do itself.
type errorReplayReadCloser struct {
	r   *bytes.Reader
	err error
}

func (e *errorReplayReadCloser) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err == io.EOF && n == 0 {
		return 0, e.err
	}
	return n, err
}

func (e *errorReplayReadCloser) Close() error { return nil }

// debugLoggingEnabled reports whether Terraform's configured log level would
// actually surface tflog.Debug output, mirroring the precedence
// terraform-plugin-log itself applies (TF_LOG_PROVIDER overrides TF_LOG for
// provider logs). It exists so the logging transport can skip the extra body
// buffering/copying work entirely when that output wouldn't be emitted.
func debugLoggingEnabled() bool {
	for _, key := range []string{"TF_LOG_PROVIDER", "TF_LOG"} {
		level, ok := os.LookupEnv(key)
		if !ok || level == "" {
			continue
		}
		switch strings.ToUpper(level) {
		case "TRACE", "DEBUG", "JSON":
			return true
		default:
			return false
		}
	}
	return false
}

// redactURL returns u's string form with any embedded userinfo password
// replaced, since url.URL.String() serializes it in plaintext (e.g. a
// cluster URL configured as https://user:password@host).
func redactURL(u *url.URL) string {
	return u.Redacted()
}

func redactHeaders(headers http.Header) map[string]string {
	redacted := make(map[string]string, len(headers))
	for name, values := range headers {
		if sensitiveHeaders[strings.ToLower(name)] {
			redacted[name] = "REDACTED"
			continue
		}
		redacted[name] = strings.Join(values, ", ")
	}
	return redacted
}

// redactBody returns a JSON request/response body with sensitive field
// values (see sensitiveBodyFields) replaced before logging. If the body
// isn't valid JSON, its raw content is never logged.
func redactBody(body []byte) string {
	var parsed interface{}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&parsed); err != nil || dec.More() {
		return "<non-JSON body omitted>"
	}

	redactJSONValue(parsed)

	redacted, err := json.Marshal(parsed)
	if err != nil {
		return "<body omitted: failed to re-encode after redaction>"
	}
	return string(redacted)
}

func redactJSONValue(v interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		for key, child := range val {
			if sensitiveBodyFields[strings.ToLower(key)] {
				val[key] = "REDACTED"
				continue
			}
			redactJSONValue(child)
		}
	case []interface{}:
		for _, child := range val {
			redactJSONValue(child)
		}
	}
}
