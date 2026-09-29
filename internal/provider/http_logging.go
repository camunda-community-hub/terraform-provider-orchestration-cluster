package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
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
// orchestration cluster API.
type loggingHTTPClient struct {
	inner camunda.HttpRequestDoer
}

func newLoggingHTTPClient(inner camunda.HttpRequestDoer) camunda.HttpRequestDoer {
	return &loggingHTTPClient{inner: inner}
}

func (c *loggingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	fields := map[string]interface{}{
		"http_method":  req.Method,
		"http_url":     req.URL.String(),
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
			"http_url":    req.URL.String(),
			"error":       err.Error(),
		})
		return resp, err
	}

	respFields := map[string]interface{}{
		"http_method":  req.Method,
		"http_url":     req.URL.String(),
		"http_status":  resp.Status,
		"http_headers": redactHeaders(resp.Header),
	}
	if resp.Body != nil {
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		if len(body) > 0 {
			respFields["http_response_body"] = redactBody(body)
		}
	}
	tflog.Debug(ctx, "received HTTP response from orchestration cluster API", respFields)

	return resp, nil
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
	if err := json.Unmarshal(body, &parsed); err != nil {
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
