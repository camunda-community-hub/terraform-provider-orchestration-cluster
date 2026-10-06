package provider

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
	"weak"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

const (
	// minConsistencyTimeout is the smallest accepted consistency_timeout. The polling timeout
	// also covers the fixed Delay (1s) before the first read, and a poll can take up to
	// MinTimeout (2s), so anything lower could expire before a single read completes.
	minConsistencyTimeout = 5 * time.Second

	// maxConsistencyTimeout is the largest accepted consistency_timeout. It keeps a mistyped
	// value from stalling a run for hours while still leaving room for very slow clusters.
	maxConsistencyTimeout = 10 * time.Minute
)

// clientConsistencyTimeouts maps each configured client to the consistency timeout of the
// provider instance that created it. Terraform runs one provider instance per provider block
// (including aliases), each with its own client, so keying by client keeps the setting
// per-instance without threading it through every resource and data source.
//
// Keys are weak pointers and entries are removed once the client is garbage collected, so the
// registry never keeps a client (or the credentials its request editors capture) alive.
var clientConsistencyTimeouts sync.Map

// registerConsistencyTimeout records the consistency timeout of the provider instance that
// owns client.
func registerConsistencyTimeout(client *camunda.ClientWithResponses, timeout time.Duration) {
	key := weak.Make(client)
	clientConsistencyTimeouts.Store(key, timeout)
	runtime.AddCleanup(client, func(key weak.Pointer[camunda.ClientWithResponses]) {
		clientConsistencyTimeouts.Delete(key)
	}, key)
}

// consistencyTimeoutFor returns the consistency timeout of the provider instance that owns
// client, or the default when client is nil or was not configured through the provider.
func consistencyTimeoutFor(client *camunda.ClientWithResponses) time.Duration {
	if client != nil {
		if timeout, ok := clientConsistencyTimeouts.Load(weak.Make(client)); ok {
			if d, ok := timeout.(time.Duration); ok {
				return d
			}
		}
	}

	return consistencyPolling.Timeout
}

// parseConsistencyTimeout parses a Go duration string and checks it against the accepted
// bounds.
func parseConsistencyTimeout(value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid duration, use a Go duration string such as \"30s\" or \"2m\": %w", value, err)
	}

	if d < minConsistencyTimeout || d > maxConsistencyTimeout {
		return 0, fmt.Errorf("%q is out of range, it must be between %s and %s", value, minConsistencyTimeout, maxConsistencyTimeout)
	}

	return d, nil
}

// consistencyTimeoutValidator validates the consistency_timeout attribute with
// parseConsistencyTimeout so that bad values fail at validate time, not mid-apply.
type consistencyTimeoutValidator struct{}

var _ validator.String = consistencyTimeoutValidator{}

func (consistencyTimeoutValidator) Description(context.Context) string {
	return fmt.Sprintf("value must be a Go duration string between %s and %s", minConsistencyTimeout, maxConsistencyTimeout)
}

func (v consistencyTimeoutValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (consistencyTimeoutValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if _, err := parseConsistencyTimeout(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid consistency_timeout", err.Error())
	}
}

// consistencyTimeoutFromConfig resolves the configured value, falling back to the default
// when it is unset.
func consistencyTimeoutFromConfig(value types.String) (time.Duration, error) {
	if value.IsNull() {
		return consistencyPolling.Timeout, nil
	}

	return parseConsistencyTimeout(value.ValueString())
}
