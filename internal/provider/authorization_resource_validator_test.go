package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// newAuthorizationValidatorTestConfig builds a tfsdk.Config for the real
// AuthorizationResource schema, with resource_id and resource_property_name set to the
// given values (nil meaning null, and unknown for a *string pointing at the sentinel
// unknownValue). It exists so mutuallyExclusiveStringValidator can be tested against
// req.Config.GetAttribute, which needs a real Config backed by the resource's own schema
// rather than a hand-rolled one that could drift from it.
func newAuthorizationValidatorTestConfig(t *testing.T, resourceId, resourcePropertyName *string) tfsdk.Config {
	t.Helper()

	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	(&AuthorizationResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building schema: %v", schemaResp.Diagnostics)
	}

	objType := schemaResp.Schema.Type().TerraformType(ctx)

	permissionsValue := tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, []tftypes.Value{
		tftypes.NewValue(tftypes.String, "READ_PROCESS_DEFINITION"),
	})

	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"id":                     tftypes.NewValue(tftypes.String, nil),
		"owner_type":             tftypes.NewValue(tftypes.String, "USER"),
		"owner_id":               tftypes.NewValue(tftypes.String, "demo"),
		"resource_type":          tftypes.NewValue(tftypes.String, "PROCESS_DEFINITION"),
		"permissions":            permissionsValue,
		"resource_id":            stringOrNullTFValue(resourceId),
		"resource_property_name": stringOrNullTFValue(resourcePropertyName),
	})

	return tfsdk.Config{Raw: raw, Schema: schemaResp.Schema}
}

// unknownValue is a sentinel passed to newAuthorizationValidatorTestConfig to request an
// unknown (rather than null or a concrete value) attribute value.
var unknownValue = "unknown"

func stringOrNullTFValue(s *string) tftypes.Value {
	switch {
	case s == nil:
		return tftypes.NewValue(tftypes.String, nil)
	case *s == unknownValue:
		return tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	default:
		return tftypes.NewValue(tftypes.String, *s)
	}
}

func strPtr(s string) *string { return &s }

// TestMutuallyExclusiveStringValidator_ValidateString is a pure unit test for
// mutuallyExclusiveStringValidator: it requires no acceptance-test setup (TF_ACC, a live
// cluster) since it only exercises the validation logic directly, including the
// cross-attribute lookup via req.Config.GetAttribute that distinguishes this validator from
// the single-value validators on TenantResource.
func TestMutuallyExclusiveStringValidator_ValidateString(t *testing.T) {
	tests := []struct {
		name                 string
		resourceId           *string
		resourcePropertyName *string
		validatedAttribute   string // "resource_id" or "resource_property_name"
		wantErr              bool
	}{
		{
			name:               "only resource_id set",
			resourceId:         strPtr("my-process"),
			validatedAttribute: "resource_id",
			wantErr:            false,
		},
		{
			name:                 "only resource_property_name set",
			resourcePropertyName: strPtr("myProperty"),
			validatedAttribute:   "resource_property_name",
			wantErr:              false,
		},
		{
			name:                 "both set, validating resource_id",
			resourceId:           strPtr("my-process"),
			resourcePropertyName: strPtr("myProperty"),
			validatedAttribute:   "resource_id",
			wantErr:              true,
		},
		{
			name:                 "both set, validating resource_property_name",
			resourceId:           strPtr("my-process"),
			resourcePropertyName: strPtr("myProperty"),
			validatedAttribute:   "resource_property_name",
			wantErr:              true,
		},
		{
			name:               "neither set",
			validatedAttribute: "resource_id",
			wantErr:            false,
		},
		{
			name:                 "resource_id unknown, resource_property_name set",
			resourceId:           &unknownValue,
			resourcePropertyName: strPtr("myProperty"),
			validatedAttribute:   "resource_property_name",
			wantErr:              false,
		},
		{
			name:                 "resource_property_name empty string, resource_id set",
			resourceId:           strPtr("my-process"),
			resourcePropertyName: strPtr(""),
			validatedAttribute:   "resource_id",
			wantErr:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newAuthorizationValidatorTestConfig(t, tt.resourceId, tt.resourcePropertyName)

			var configValue types.String
			var validatedPath, otherPath path.Path
			switch tt.validatedAttribute {
			case "resource_id":
				validatedPath = path.Root("resource_id")
				otherPath = path.Root("resource_property_name")
				configValue = stringOrNullTypesValue(tt.resourceId)
			case "resource_property_name":
				validatedPath = path.Root("resource_property_name")
				otherPath = path.Root("resource_id")
				configValue = stringOrNullTypesValue(tt.resourcePropertyName)
			default:
				t.Fatalf("unknown validatedAttribute %q", tt.validatedAttribute)
			}

			req := validator.StringRequest{
				Path:        validatedPath,
				Config:      cfg,
				ConfigValue: configValue,
			}
			resp := &validator.StringResponse{}

			mutuallyExclusiveStringValidator{otherAttribute: otherPath}.ValidateString(context.Background(), req, resp)

			gotErr := resp.Diagnostics.HasError()
			if gotErr != tt.wantErr {
				t.Fatalf("got error = %v, want error = %v (diagnostics: %v)", gotErr, tt.wantErr, resp.Diagnostics)
			}
		})
	}
}

func stringOrNullTypesValue(s *string) types.String {
	switch {
	case s == nil:
		return types.StringNull()
	case *s == unknownValue:
		return types.StringUnknown()
	default:
		return types.StringValue(*s)
	}
}
