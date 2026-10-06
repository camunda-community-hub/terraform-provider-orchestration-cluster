package provider

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

func TestParseConsistencyTimeout(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr string
	}{
		{in: "30s", want: 30 * time.Second},
		{in: "5s", want: 5 * time.Second},
		{in: "2m", want: 2 * time.Minute},
		{in: "1m30s", want: 90 * time.Second},
		{in: "10m", want: 10 * time.Minute},
		{in: "", wantErr: "not a valid duration"},
		{in: "soon", wantErr: "not a valid duration"},
		{in: "30", wantErr: "not a valid duration"},
		{in: "0s", wantErr: "out of range"},
		{in: "-5s", wantErr: "out of range"},
		{in: "4999ms", wantErr: "out of range"},
		{in: "1s", wantErr: "out of range"},
		{in: "10m1s", wantErr: "out of range"},
		{in: "24h", wantErr: "out of range"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseConsistencyTimeout(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestConsistencyTimeoutValidator(t *testing.T) {
	tests := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{name: "valid", value: types.StringValue("45s")},
		{name: "null", value: types.StringNull()},
		{name: "unknown", value: types.StringUnknown()},
		{name: "malformed", value: types.StringValue("abc"), wantErr: true},
		{name: "too small", value: types.StringValue("4s"), wantErr: true},
		{name: "too large", value: types.StringValue("1h"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			consistencyTimeoutValidator{}.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("consistency_timeout"),
				ConfigValue: tt.value,
			}, resp)

			if got := resp.Diagnostics.HasError(); got != tt.wantErr {
				t.Fatalf("HasError() = %v, want %v: %v", got, tt.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestProviderConfigure_ConsistencyTimeout(t *testing.T) {
	timeoutOf := func(t *testing.T, vals map[string]tftypes.Value) time.Duration {
		t.Helper()
		resp := configureProvider(t, vals)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		client, ok := resp.ResourceData.(*camunda.ClientWithResponses)
		if !ok {
			t.Fatalf("ResourceData is %T, want *ClientWithResponses", resp.ResourceData)
		}
		return consistencyTimeoutFor(client)
	}
	url := tftypes.NewValue(tftypes.String, "http://localhost:1")

	t.Run("default", func(t *testing.T) {
		if got := timeoutOf(t, map[string]tftypes.Value{"url": url}); got != 30*time.Second {
			t.Fatalf("got %s, want 30s", got)
		}
	})

	t.Run("configured", func(t *testing.T) {
		got := timeoutOf(t, map[string]tftypes.Value{
			"url":                 url,
			"consistency_timeout": tftypes.NewValue(tftypes.String, "2m"),
		})
		if got != 2*time.Minute {
			t.Fatalf("got %s, want 2m", got)
		}
	})

	t.Run("aliases keep their own value", func(t *testing.T) {
		slow := timeoutOf(t, map[string]tftypes.Value{
			"url":                 url,
			"consistency_timeout": tftypes.NewValue(tftypes.String, "5m"),
		})
		fast := timeoutOf(t, map[string]tftypes.Value{
			"url":                 url,
			"consistency_timeout": tftypes.NewValue(tftypes.String, "5s"),
		})
		def := timeoutOf(t, map[string]tftypes.Value{"url": url})
		if slow != 5*time.Minute || fast != 5*time.Second || def != 30*time.Second {
			t.Fatalf("got slow=%s fast=%s default=%s, want 5m 5s 30s", slow, fast, def)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		for _, bad := range []string{"later", "0s", "24h"} {
			resp := configureProvider(t, map[string]tftypes.Value{
				"url":                 url,
				"consistency_timeout": tftypes.NewValue(tftypes.String, bad),
			})
			if !resp.Diagnostics.HasError() {
				t.Fatalf("%q: expected an error diagnostic", bad)
			}
			if resp.ResourceData != nil {
				t.Fatalf("%q: no client must be provided for an invalid timeout", bad)
			}
		}
	})
}

func TestConsistencyTimeoutFor_DefaultsWithoutRegistration(t *testing.T) {
	if got := consistencyTimeoutFor(nil); got != consistencyPolling.Timeout {
		t.Fatalf("nil client: got %s, want %s", got, consistencyPolling.Timeout)
	}

	client, err := camunda.NewClientWithResponses("http://localhost:1")
	if err != nil {
		t.Fatal(err)
	}
	if got := consistencyTimeoutFor(client); got != consistencyPolling.Timeout {
		t.Fatalf("unregistered client: got %s, want %s", got, consistencyPolling.Timeout)
	}
}

func TestWaitForConsistency_HonorsPerClientTimeout(t *testing.T) {
	withFastConsistencyPolling(t)

	newClient := func(timeout time.Duration) *camunda.ClientWithResponses {
		client, err := camunda.NewClientWithResponses("http://localhost:1")
		if err != nil {
			t.Fatal(err)
		}
		registerConsistencyTimeout(client, timeout)
		return client
	}
	short := newClient(200 * time.Millisecond)
	long := newClient(5 * time.Second)

	// Becomes consistent after 600ms: past the short client's budget, within the long one's.
	readyAt := time.Now().Add(600 * time.Millisecond)
	refresh := func() (string, bool, error) {
		return "v", !time.Now().Before(readyAt), nil
	}

	start := time.Now()
	if _, err := waitForConsistency(context.Background(), short, "widget", refresh); err == nil {
		t.Fatal("short timeout: expected a timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 550*time.Millisecond {
		t.Fatalf("short timeout took %s, want it to give up near 200ms", elapsed)
	}

	if _, err := waitForConsistency(context.Background(), long, "widget", refresh); err != nil {
		t.Fatalf("long timeout: unexpected error: %s", err)
	}
}
