package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// descriptionAttributeMarkdown returns the shared MarkdownDescription text for a
// resource's `description` attribute, parameterized by the entity noun (e.g. "group",
// "role", "tenant"). The API cannot distinguish an empty string from an absent
// description, so an explicitly configured empty string is rejected rather than
// silently normalized to null.
func descriptionAttributeMarkdown(entity string) string {
	return fmt.Sprintf("The description of the %s. Omit this attribute (or set it to `null`) to indicate no "+
		"description — the API cannot distinguish an empty string from an absent description, so an explicitly "+
		"configured empty string is rejected rather than silently normalized to null.", entity)
}

// nonEmptyStringValidator rejects an explicitly configured empty string, while still
// allowing null (attribute omitted) and unknown values through. It exists because the
// description-bearing entities (group, role, tenant) conflate an empty description
// with an absent one (see optionalStringValue): without this validator, a configured
// `description = ""` would be silently normalized to null after Create/Update,
// producing a persistent diff between the configured value and the stored state.
type nonEmptyStringValidator struct{}

func (v nonEmptyStringValidator) Description(ctx context.Context) string {
	return "must not be an empty string; omit the attribute (or set it to null) instead"
}

func (v nonEmptyStringValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v nonEmptyStringValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if req.ConfigValue.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Description",
			fmt.Sprintf("%s must not be an empty string; omit the attribute (or set it to null) to indicate no description.", req.Path),
		)
	}
}

// optionalStringValue converts an optional *string, as returned by the API for a
// description field, into a types.String. The API reports a cleared or never-set
// description as an empty string rather than omitting the field or returning null,
// so both a nil pointer and an empty string must map to a null value: otherwise a
// config with no `description` would show a perpetual diff against an API-reported
// empty string.
func optionalStringValue(s *string) types.String {
	if s == nil || *s == "" {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// normalizedDescription treats a nil description pointer the same as an empty
// string, matching the API's behavior of reporting a cleared or never-set
// description as "" rather than omitting it or returning null.
func normalizedDescription(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
