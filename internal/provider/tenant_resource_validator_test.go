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
// validation logic directly. It covers the maximum accepted length (31, matching the
// Zeebe gateway's tenant ID validation used for process orchestration operations, which
// is stricter than the tenant-creation REST API's own 256-character/`+`/`@`-inclusive
// contract), one character over that boundary, and the full set of characters this
// stricter rule allows (letters, digits, '_', '-', '.') as well as ones it now rejects
// ('+', '@') despite the creation endpoint itself accepting them.
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
			name:    "max length accepted (31 chars)",
			value:   strings.Repeat("a", 31),
			wantErr: false,
		},
		{
			name:    "one character over max length (32 chars)",
			value:   strings.Repeat("a", 32),
			wantErr: true,
		},
		{
			name:    "plus sign rejected despite creation API allowing it",
			value:   "tenant+1",
			wantErr: true,
		},
		{
			name:    "at sign rejected despite creation API allowing it",
			value:   "tenant@example",
			wantErr: true,
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

// TestNonEmptyStringValidator_ValidateString is a pure unit test for nonEmptyStringValidator,
// used on the tenant `description` attribute to reject an explicitly configured empty string
// (which the API would otherwise silently normalize to null, per optionalStringValue) while
// still allowing null and unknown values through.
func TestNonEmptyStringValidator_ValidateString(t *testing.T) {
	tests := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{
			name:    "non-empty string accepted",
			value:   types.StringValue("A test tenant"),
			wantErr: false,
		},
		{
			name:    "empty string rejected",
			value:   types.StringValue(""),
			wantErr: true,
		},
		{
			name:    "null accepted (attribute omitted)",
			value:   types.StringNull(),
			wantErr: false,
		},
		{
			name:    "unknown accepted",
			value:   types.StringUnknown(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("description"),
				ConfigValue: tt.value,
			}
			resp := &validator.StringResponse{}

			nonEmptyStringValidator{}.ValidateString(context.Background(), req, resp)

			gotErr := resp.Diagnostics.HasError()
			if gotErr != tt.wantErr {
				t.Fatalf("value %q: got error = %v, want error = %v (diagnostics: %v)", tt.value, gotErr, tt.wantErr, resp.Diagnostics)
			}
		})
	}
}
