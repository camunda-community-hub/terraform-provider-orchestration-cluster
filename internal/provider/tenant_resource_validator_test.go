package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestTenantIdValidator_ValidateString is a pure unit test for tenantIdValidator: it
// requires no acceptance-test setup (TF_ACC, a live cluster) since it only exercises the
// validation logic directly. It covers the maximum accepted length (256, per the
// TenantId schema's maxLength in spec/8.9/bundled-api.yaml), one character over that
// boundary, and the full set of characters the same schema's `pattern` field allows
// (letters, digits, '_', '-', '+', '.', '@').
func TestTenantIdValidator_ValidateString(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:    "typical id",
			value:   "tenant1",
			wantErr: false,
		},
		{
			name:    "max length accepted (256 chars)",
			value:   strings.Repeat("a", 256),
			wantErr: false,
		},
		{
			name:    "one character over max length (257 chars)",
			value:   strings.Repeat("a", 257),
			wantErr: true,
		},
		{
			name:    "plus sign accepted per API pattern",
			value:   "tenant+1",
			wantErr: false,
		},
		{
			name:    "at sign accepted per API pattern",
			value:   "tenant@example",
			wantErr: false,
		},
		{
			name:    "dot and underscore and hyphen accepted",
			value:   "tenant_1-2.3",
			wantErr: false,
		},
		{
			name:    "space rejected",
			value:   "tenant 1",
			wantErr: true,
		},
		{
			name:    "empty string rejected",
			value:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("tenant_id"),
				ConfigValue: types.StringValue(tt.value),
			}
			resp := &validator.StringResponse{}

			tenantIdValidator{}.ValidateString(context.Background(), req, resp)

			gotErr := resp.Diagnostics.HasError()
			if gotErr != tt.wantErr {
				t.Fatalf("value %q: got error = %v, want error = %v (diagnostics: %v)", tt.value, gotErr, tt.wantErr, resp.Diagnostics)
			}
		})
	}
}

// TestTenantIdValidator_ValidateString_NullAndUnknown ensures the validator skips
// validation for null/unknown config values, letting the `Required` schema constraint
// (and Terraform's own unknown-value handling) do their jobs instead.
func TestTenantIdValidator_ValidateString_NullAndUnknown(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value types.String
	}{
		{name: "null", value: types.StringNull()},
		{name: "unknown", value: types.StringUnknown()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("tenant_id"),
				ConfigValue: tt.value,
			}
			resp := &validator.StringResponse{}

			tenantIdValidator{}.ValidateString(context.Background(), req, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("expected no error for %s value, got: %v", tt.name, resp.Diagnostics)
			}
		})
	}
}
