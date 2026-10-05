package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestAuthorizationKeyValidator_ValidateString is a pure unit test for
// authorizationKeyValidator covering the LongKey contract: an optionally negative
// integer string of 1 to 25 characters, null and unknown values being skipped.
func TestAuthorizationKeyValidator_ValidateString(t *testing.T) {
	tests := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{name: "typical key", value: types.StringValue("2251799813685249")},
		{name: "negative key", value: types.StringValue("-42")},
		{name: "max length (25 chars)", value: types.StringValue(strings.Repeat("1", 25))},
		{name: "one over max length (26 chars)", value: types.StringValue(strings.Repeat("1", 26)), wantErr: true},
		{name: "empty string", value: types.StringValue(""), wantErr: true},
		{name: "non-numeric", value: types.StringValue("abc"), wantErr: true},
		{name: "lone minus sign", value: types.StringValue("-"), wantErr: true},
		{name: "null skipped", value: types.StringNull()},
		{name: "unknown skipped", value: types.StringUnknown()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validator.StringRequest{Path: path.Root("id"), ConfigValue: tt.value}
			resp := &validator.StringResponse{}

			authorizationKeyValidator{}.ValidateString(context.Background(), req, resp)

			if gotErr := resp.Diagnostics.HasError(); gotErr != tt.wantErr {
				t.Fatalf("value %v: got error = %v, want %v (diagnostics: %v)", tt.value, gotErr, tt.wantErr, resp.Diagnostics)
			}
		})
	}
}
