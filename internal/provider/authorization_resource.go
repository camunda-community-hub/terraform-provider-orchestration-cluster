package provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var _ resource.Resource = &AuthorizationResource{}
var _ resource.ResourceWithImportState = &AuthorizationResource{}

func NewAuthorizationResource() resource.Resource {
	return &AuthorizationResource{}
}

type AuthorizationResource struct {
	client *camunda.ClientWithResponses
}

type AuthorizationResourceModel struct {
	Id                   types.String `tfsdk:"id"`
	OwnerType            types.String `tfsdk:"owner_type"`
	OwnerId              types.String `tfsdk:"owner_id"`
	ResourceType         types.String `tfsdk:"resource_type"`
	PermissionTypes      types.Set    `tfsdk:"permission_types"`
	ResourceId           types.String `tfsdk:"resource_id"`
	ResourcePropertyName types.String `tfsdk:"resource_property_name"`
}

func (r *AuthorizationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_authorization"
}

func (r *AuthorizationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Camunda cluster authorization",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique key of the authorization (string-encoded int64).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"owner_type": schema.StringAttribute{
				MarkdownDescription: "The type of the owner of permissions.",
				Required:            true,
			},
			"owner_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the owner of permissions.",
				Required:            true,
			},
			"resource_type": schema.StringAttribute{
				MarkdownDescription: "The type of resource that the permissions relate to.",
				Required:            true,
			},
			"permission_types": schema.SetAttribute{
				MarkdownDescription: "The permission types.",
				Required:            true,
				ElementType:         types.StringType,
			},
			"resource_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the resource the permission relates to. Use \"*\" to match all resources. " +
					"Mutually exclusive with `resource_property_name`. If neither is set, defaults to \"*\".",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					authorizationScopePlanModifier{siblingAttribute: path.Root("resource_property_name"), defaultValue: "*"},
				},
				Validators: []validator.String{
					nonEmptyScopeValidator{},
					mutuallyExclusiveStringValidator{otherAttribute: path.Root("resource_property_name")},
				},
			},
			"resource_property_name": schema.StringAttribute{
				MarkdownDescription: "The name of the resource property the permission relates to. Mutually exclusive with " +
					"`resource_id`. If neither is set, `resource_id` defaults to \"*\".",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					authorizationScopePlanModifier{siblingAttribute: path.Root("resource_id"), defaultValue: noDefaultScopeValue},
				},
				Validators: []validator.String{
					nonEmptyScopeValidator{},
					mutuallyExclusiveStringValidator{otherAttribute: path.Root("resource_id")},
				},
			},
		},
	}
}

// mutuallyExclusiveStringValidator rejects a configured value when another string
// attribute is also configured (non-null, non-unknown, non-empty), used to enforce that
// `resource_id` and `resource_property_name` are not both set: the API's
// AuthorizationRequest accepts either an ID-based or a property-based authorization, never
// both at once.
type mutuallyExclusiveStringValidator struct {
	otherAttribute path.Path
}

func (v mutuallyExclusiveStringValidator) Description(ctx context.Context) string {
	return fmt.Sprintf("cannot be set at the same time as %s", v.otherAttribute)
}

func (v mutuallyExclusiveStringValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v mutuallyExclusiveStringValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || req.ConfigValue.ValueString() == "" {
		return
	}

	var other types.String
	diags := req.Config.GetAttribute(ctx, v.otherAttribute, &other)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	if other.IsNull() || other.IsUnknown() || other.ValueString() == "" {
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Conflicting Attributes",
		fmt.Sprintf("%s and %s are mutually exclusive; set at most one of them.", req.Path, v.otherAttribute),
	)
}

// nonEmptyScopeValidator rejects an explicitly configured empty string on resource_id or
// resource_property_name, while still allowing null (attribute omitted) and unknown values
// through. It exists so an empty string configured for one of these scope attributes isn't
// silently treated as "unset" by resolveAuthorizationRequestVariant. Unlike
// nonEmptyStringValidator (tenant_resource.go), which hardcodes a description-specific
// diagnostic, this validator derives its message from req.Path so it reads correctly on
// whichever of the two attributes it is attached to.
type nonEmptyScopeValidator struct{}

func (v nonEmptyScopeValidator) Description(ctx context.Context) string {
	return "must not be an empty string; omit the attribute (or set it to null) instead"
}

func (v nonEmptyScopeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v nonEmptyScopeValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if req.ConfigValue.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Value",
			fmt.Sprintf("%s must not be an empty string; omit the attribute (or set it to null) instead.", req.Path),
		)
	}
}

// noDefaultScopeValue is the defaultValue sentinel for a scope attribute that has no
// non-null default (resource_property_name): only req.StateValue.IsNull() counts as "already
// at default" for such an attribute, since it never has a meaningful non-null default value
// the way resource_id has "*".
const noDefaultScopeValue = ""

// authorizationScopePlanModifier implements the cross-attribute plan-modifier logic shared by
// resource_id and resource_property_name. Both are Optional+Computed and mutually exclusive,
// so a bare stringplanmodifier.UseStateForUnknown() on each would restore the OMITTED
// attribute's prior state value even when the sibling attribute's config just changed to fill
// the other scope, producing an inconsistent plan where both scopes appear populated (and
// causing resolveAuthorizationRequestVariant to resolve the wrong variant in one of the two
// transition directions). This modifier instead only restores prior state when neither
// attribute is configured AND the prior state already represents this attribute's own
// "unused/default" state (see defaultValue below) -- meaning nothing about scope is actually
// changing. When this attribute is omitted but the sibling IS configured, it plans an
// explicit null so the abandoned scope's stale value doesn't linger in the plan. When both
// attributes are omitted but the prior state held a real, explicit value for this attribute
// (i.e. removing a previously configured scope to fall back to the documented default), it
// plans unknown so Create/Update resolves the true default and the plan doesn't silently keep
// the removed scope.
type authorizationScopePlanModifier struct {
	siblingAttribute path.Path

	// defaultValue is this attribute's own "value when unused" -- "*" for resource_id, or
	// noDefaultScopeValue (empty string sentinel) for resource_property_name, which has no
	// non-null default and is only ever "unused" when null.
	defaultValue string
}

func (m authorizationScopePlanModifier) Description(ctx context.Context) string {
	return fmt.Sprintf("Preserves the prior state value when this attribute and %s are both omitted from "+
		"configuration and the prior state already reflects this attribute's default (unused) value. When this "+
		"attribute is omitted but %s is configured, plans an explicit null instead, so switching between the two "+
		"scopes doesn't retain a stale value from the abandoned scope. When both are omitted but the prior state "+
		"held a real, explicit value for this attribute, plans unknown so the documented default is resolved "+
		"during apply instead of preserving the removed scope.",
		m.siblingAttribute, m.siblingAttribute)
}

func (m authorizationScopePlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m authorizationScopePlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Do nothing if there is no state (resource is being created): defaulting resource_id to
	// "*" (or resolving the property-based variant) happens in Create() itself, so the plan
	// value should stay unknown, matching stringplanmodifier.UseStateForUnknown's own guard.
	if req.State.Raw.IsNull() {
		return
	}

	// Likewise do nothing during destroy: state is non-null there, but plan is null, and
	// this modifier must not replace that null destroy plan with a restored/unknown value.
	if req.Plan.Raw.IsNull() {
		return
	}

	// A value explicitly configured for this attribute flows through unchanged; only react
	// when the attribute itself is omitted from configuration.
	if !req.ConfigValue.IsNull() {
		return
	}

	// Deliberately NOT bailing out here just because req.PlanValue is already known: for an
	// Optional+Computed attribute the framework can prefill the proposed plan directly from
	// prior state before this modifier ever runs, so a known PlanValue does not mean "nothing
	// to do" -- it can be exactly the stale, abandoned-scope value this modifier exists to
	// correct. Always check the sibling below whenever this attribute is omitted from config,
	// regardless of what PlanValue currently holds.

	var siblingConfigValue types.String
	diags := req.Config.GetAttribute(ctx, m.siblingAttribute, &siblingConfigValue)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	if siblingConfigValue.IsUnknown() {
		// The sibling's config value isn't known yet at plan time (e.g. it's derived from
		// another resource's not-yet-known attribute). We can't yet tell whether it will
		// resolve to null (in which case this attribute should take over the default) or to
		// a real value (in which case this attribute should become null) -- deciding either
		// way now risks Terraform's "inconsistent result after apply" once the sibling's
		// real value is known. Leave this attribute unknown too, so both resolve together at
		// apply time.
		resp.PlanValue = types.StringUnknown()
		return
	}

	if siblingConfigValue.IsNull() {
		// Neither scope attribute is configured. If the prior state for THIS attribute already
		// reflects its own "unused/default" value, nothing about scope is actually changing, so
		// behave like UseStateForUnknown and restore the prior state value.
		//
		// For an attribute with no non-null default (resource_property_name, defaultValue ==
		// noDefaultScopeValue), null IS that default: it's a stable "unused" state regardless of
		// what the sibling attribute is doing. For an attribute WITH a non-null default
		// (resource_id, defaultValue "*"), a null state does NOT count as already-default: given
		// resourceId/resourcePropertyName are mutually exclusive and one of them always resolves
		// to a concrete value, resource_id can only be null in state because the sibling
		// (resource_property_name) was previously active -- i.e. a real scope switch away from
		// property-based is in progress, not a no-op. Only an already-concrete "*" is a true no-op
		// for resource_id.
		var alreadyAtDefault bool
		if m.defaultValue == noDefaultScopeValue {
			alreadyAtDefault = req.StateValue.IsNull()
		} else {
			alreadyAtDefault = !req.StateValue.IsNull() && req.StateValue.ValueString() == m.defaultValue
		}
		if alreadyAtDefault {
			resp.PlanValue = req.StateValue
			return
		}

		// The prior state held a real, explicit value for this attribute, and the user has now
		// removed both scope attributes from config, wanting the documented default. Restoring
		// req.StateValue here would silently keep the removed scope instead of transitioning to
		// the default, so plan unknown instead: Create/Update resolves the true default value,
		// and the subsequent consistency-wait re-read populates the correct final state.
		resp.PlanValue = types.StringUnknown()
		return
	}

	// The sibling attribute is configured: the user is switching to the other scope type, so
	// this attribute must plan to become explicitly null rather than retaining a stale value
	// from the abandoned scope.
	resp.PlanValue = types.StringNull()
}

func (r *AuthorizationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*camunda.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *camunda.ClientWithResponses, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *AuthorizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AuthorizationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var permissionStrings []string
	resp.Diagnostics.Append(data.PermissionTypes.ElementsAs(ctx, &permissionStrings, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	permissionTypes := make([]camunda.PermissionTypeEnum, len(permissionStrings))
	for i, p := range permissionStrings {
		permissionTypes[i] = camunda.PermissionTypeEnum(p)
	}

	variant, err := resolveAuthorizationRequestVariant(data.ResourceId, data.ResourcePropertyName)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Configuration", fmt.Sprintf("Unable to resolve authorization scope: %s", err))
		return
	}

	authReq, err := buildAuthorizationRequest(
		data.OwnerId.ValueString(),
		camunda.OwnerTypeEnum(data.OwnerType.ValueString()),
		permissionTypes,
		camunda.ResourceTypeEnum(data.ResourceType.ValueString()),
		variant,
	)
	if err != nil {
		resp.Diagnostics.AddError("Encoding Error", fmt.Sprintf("Unable to encode authorization request: %s", err))
		return
	}

	apiResp, err := r.client.CreateAuthorizationWithResponse(ctx, authReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create authorization, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusCreated {
		resp.Diagnostics.AddError("Not Created", fmt.Sprintf("Error while creating authorization, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 201 but with no parseable JSON body")
		return
	}

	authKey := apiResp.JSON201.AuthorizationKey
	data.Id = types.StringValue(authKey)

	// Resolve resource_id/resource_property_name from the already-computed variant rather
	// than leaving them as whatever the plan held: an omitted Optional+Computed scope
	// attribute is still unknown on the plan, and this fallback state exists precisely to
	// cover the case where the poll below times out, so it must hold known, valid values.
	if variant.isPropertyBased {
		data.ResourceId = types.StringNull()
		data.ResourcePropertyName = types.StringValue(variant.resourcePropertyName)
	} else {
		data.ResourceId = types.StringValue(variant.resourceId)
		data.ResourcePropertyName = types.StringNull()
	}

	// Persist state from the plan plus the newly-assigned key before polling for read
	// consistency, so a polling timeout or transport error doesn't orphan the authorization
	// the API already created.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Re-read the full state from the engine, waiting out the eventual consistency of
	// GetAuthorization: an immediate read right after a successful create can still 404.
	getResp, err := readAuthorizationWithRetry(ctx, r.client, authKey)
	if err != nil {
		resp.Diagnostics.AddWarning("Consistency Check Failed", fmt.Sprintf("Authorization %q was created but could not be confirmed readable yet: %s. State was saved from the create response; a later refresh will pick up any drift.", authKey, err))
		return
	}

	if getResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	var modelDiags diag.Diagnostics
	data, modelDiags = authorizationResultToModel(ctx, data.Id, getResp.JSON200)
	resp.Diagnostics.Append(modelDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created authorization resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuthorizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AuthorizationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.GetAuthorizationWithResponse(ctx, data.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read authorization '%s', got error: %s", data.Id.ValueString(), err))
		return
	}

	if apiResp.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Error while reading authorization '%s', got HTTP error: %d", data.Id.ValueString(), apiResp.StatusCode()))
		return
	}

	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	var readModelDiags diag.Diagnostics
	data, readModelDiags = authorizationResultToModel(ctx, data.Id, apiResp.JSON200)
	resp.Diagnostics.Append(readModelDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuthorizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AuthorizationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AuthorizationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var permissionStrings []string
	resp.Diagnostics.Append(data.PermissionTypes.ElementsAs(ctx, &permissionStrings, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	permissionTypes := make([]camunda.PermissionTypeEnum, len(permissionStrings))
	for i, p := range permissionStrings {
		permissionTypes[i] = camunda.PermissionTypeEnum(p)
	}

	// Use the plan's resource_id/resource_property_name, not the state's: the plan already
	// carries over an unmodified property-based authorization's resource_property_name (via
	// authorizationScopePlanModifier, since it is Optional+Computed), so an update that only
	// changes e.g. permissions still resolves to the property-based variant instead of
	// defaulting resource_id to "*" and silently converting the authorization to a wildcard
	// ID-based grant. When the plan reflects an actual scope transition, the plan modifier
	// instead plans the abandoned attribute to explicit null, so this resolves to the newly
	// configured variant.
	variant, err := resolveAuthorizationRequestVariant(data.ResourceId, data.ResourcePropertyName)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Configuration", fmt.Sprintf("Unable to resolve authorization scope: %s", err))
		return
	}

	updateReq, err := buildAuthorizationRequest(
		data.OwnerId.ValueString(),
		camunda.OwnerTypeEnum(data.OwnerType.ValueString()),
		permissionTypes,
		camunda.ResourceTypeEnum(data.ResourceType.ValueString()),
		variant,
	)
	if err != nil {
		resp.Diagnostics.AddError("Encoding Error", fmt.Sprintf("Unable to encode authorization request: %s", err))
		return
	}

	apiResp, err := r.client.UpdateAuthorizationWithResponse(ctx, state.Id.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update authorization, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Not Updated", fmt.Sprintf("Error while updating authorization, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	// Re-read to get current state, waiting until the read reflects the newly-applied
	// permissions and resource scope: an immediate read right after a successful update can
	// still return the pre-update values.
	getResp, err := readAuthorizationUntilConsistent(
		ctx,
		r.client,
		state.Id.ValueString(),
		data.OwnerId.ValueString(),
		camunda.OwnerTypeEnum(data.OwnerType.ValueString()),
		camunda.ResourceTypeEnum(data.ResourceType.ValueString()),
		permissionStrings,
		variant,
	)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to confirm authorization update, got error: %s", err))
		return
	}

	if getResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	var updateModelDiags diag.Diagnostics
	data, updateModelDiags = authorizationResultToModel(ctx, state.Id, getResp.JSON200)
	resp.Diagnostics.Append(updateModelDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuthorizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AuthorizationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.DeleteAuthorizationWithResponse(ctx, data.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete authorization, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting authorization, got HTTP error: %d", apiResp.StatusCode()))
		return
	}
}

func (r *AuthorizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Validate import ID is a valid int64
	if _, err := strconv.ParseInt(req.ID, 10, 64); err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Authorization ID must be a numeric string, got: %s", req.ID))
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// authorizationRequestVariant identifies which of the two mutually exclusive shapes an
// AuthorizationRequest should take: ID-based (scoped to a specific resource ID, or "*" for
// all resources) or property-based (scoped to a named resource property).
type authorizationRequestVariant struct {
	resourceId           string
	resourcePropertyName string
	isPropertyBased      bool
}

// resolveAuthorizationRequestVariant picks the request variant to build from a
// resource_id/resource_property_name pair as found in a Terraform plan or state: a
// non-null, non-unknown, non-empty resource_property_name selects the property-based
// variant; otherwise the ID-based variant is used, defaulting resource_id to "*" when it is
// not itself configured. The schema-level mutuallyExclusiveStringValidator only rejects both
// attributes being concretely set at validation time — it lets unknowns through, so two
// values that are both unknown during plan can still both resolve non-empty by apply time.
// This is rechecked here against the resolved values, returning an error in that case
// instead of silently picking the property-based variant and dropping resource_id.
func resolveAuthorizationRequestVariant(resourceId, resourcePropertyName types.String) (authorizationRequestVariant, error) {
	hasResourceId := !resourceId.IsNull() && !resourceId.IsUnknown() && resourceId.ValueString() != ""
	hasResourcePropertyName := !resourcePropertyName.IsNull() && !resourcePropertyName.IsUnknown() && resourcePropertyName.ValueString() != ""

	if hasResourceId && hasResourcePropertyName {
		return authorizationRequestVariant{}, fmt.Errorf("resource_id %q and resource_property_name %q both resolved to non-empty values; only one may be set", resourceId.ValueString(), resourcePropertyName.ValueString())
	}

	if hasResourcePropertyName {
		return authorizationRequestVariant{resourcePropertyName: resourcePropertyName.ValueString(), isPropertyBased: true}, nil
	}

	resolvedResourceId := "*"
	if hasResourceId {
		resolvedResourceId = resourceId.ValueString()
	}
	return authorizationRequestVariant{resourceId: resolvedResourceId}, nil
}

// buildAuthorizationRequest encodes an AuthorizationRequest for the given variant.
// UpdateAuthorizationJSONRequestBody is a type alias for camunda.AuthorizationRequest, so
// this same encoding is used for both Create and Update.
func buildAuthorizationRequest(ownerId string, ownerType camunda.OwnerTypeEnum, permissionTypes []camunda.PermissionTypeEnum, resourceType camunda.ResourceTypeEnum, variant authorizationRequestVariant) (camunda.AuthorizationRequest, error) {
	var authReq camunda.AuthorizationRequest

	if variant.isPropertyBased {
		propReq := camunda.AuthorizationPropertyBasedRequest{
			OwnerId:              ownerId,
			OwnerType:            ownerType,
			PermissionTypes:      permissionTypes,
			ResourcePropertyName: variant.resourcePropertyName,
			ResourceType:         resourceType,
		}
		return authReq, authReq.FromAuthorizationPropertyBasedRequest(propReq)
	}

	idReq := camunda.AuthorizationIdBasedRequest{
		OwnerId:         ownerId,
		OwnerType:       ownerType,
		PermissionTypes: permissionTypes,
		ResourceId:      variant.resourceId,
		ResourceType:    resourceType,
	}
	return authReq, authReq.FromAuthorizationIdBasedRequest(idReq)
}

func authorizationResultToModel(ctx context.Context, id types.String, result *camunda.AuthorizationResult) (AuthorizationResourceModel, diag.Diagnostics) {
	data := AuthorizationResourceModel{
		Id:           id,
		OwnerId:      types.StringValue(result.OwnerId),
		OwnerType:    types.StringValue(string(result.OwnerType)),
		ResourceType: types.StringValue(string(result.ResourceType)),
	}

	perms := make([]string, len(result.PermissionTypes))
	for i, p := range result.PermissionTypes {
		perms[i] = string(p)
	}
	permSet, diags := types.SetValueFrom(ctx, types.StringType, perms)
	data.PermissionTypes = permSet

	// ResourceId and ResourcePropertyName are mutually exclusive on the API side: a
	// property-based authorization has ResourcePropertyName set and ResourceId nil. Defaulting
	// resource_id to "*" for that case would misrepresent the authorization's actual scope, and
	// a later Update would then replace it with a much-too-broad wildcard grant.
	switch {
	case result.ResourcePropertyName != nil && *result.ResourcePropertyName != "":
		data.ResourcePropertyName = types.StringValue(*result.ResourcePropertyName)
		data.ResourceId = types.StringNull()
	case result.ResourceId != nil && *result.ResourceId != "":
		data.ResourceId = types.StringValue(*result.ResourceId)
		data.ResourcePropertyName = types.StringNull()
	default:
		data.ResourceId = types.StringValue("*")
		data.ResourcePropertyName = types.StringNull()
	}

	return data, diags
}

// readAuthorizationWithRetry handles the eventual consistency of fetching an authorization by
// retrying a few times: if an authorization was just created, it may not be immediately
// available through the API.
func readAuthorizationWithRetry(ctx context.Context, client *camunda.ClientWithResponses, authKey string) (*camunda.GetAuthorizationResponse, error) {
	return waitForConsistency(ctx, fmt.Sprintf("authorization %q", authKey), func() (*camunda.GetAuthorizationResponse, bool, error) {
		readResp, err := client.GetAuthorizationWithResponse(ctx, authKey)
		if err != nil {
			return nil, false, err
		}

		switch readResp.StatusCode() {
		case http.StatusOK:
			return readResp, true, nil
		case http.StatusNotFound:
			return readResp, false, nil
		default:
			return nil, false, fmt.Errorf("got HTTP error: %d: %s", readResp.StatusCode(), readResp.Body)
		}
	})
}

// readAuthorizationUntilConsistent polls GET until it reflects the given owner, resource
// type, permissions, and resource scope (resource ID or resource property name, depending
// on the expected variant) -- every field Update() can actually change -- handling the same
// eventual consistency on updates as readAuthorizationWithRetry does on creates: the
// read-side projection can briefly return the pre-update values right after a successful
// update, which would otherwise make Terraform's post-apply refresh plan non-empty. Checking
// only a subset of the updated fields would let a stale read that happens to match on those
// fields be accepted while others (e.g. owner_id changed but permissions didn't) are still
// pre-update.
func readAuthorizationUntilConsistent(ctx context.Context, client *camunda.ClientWithResponses, authKey string, expectedOwnerId string, expectedOwnerType camunda.OwnerTypeEnum, expectedResourceType camunda.ResourceTypeEnum, expectedPermissions []string, expectedVariant authorizationRequestVariant) (*camunda.GetAuthorizationResponse, error) {
	return waitForConsistency(ctx, fmt.Sprintf("authorization %q", authKey), func() (*camunda.GetAuthorizationResponse, bool, error) {
		readResp, err := client.GetAuthorizationWithResponse(ctx, authKey)
		if err != nil {
			return nil, false, err
		}

		if readResp.StatusCode() == http.StatusNotFound {
			return readResp, false, nil
		}

		if readResp.StatusCode() != http.StatusOK {
			return nil, false, fmt.Errorf("got HTTP error: %d: %s", readResp.StatusCode(), readResp.Body)
		}

		if readResp.JSON200 == nil {
			return readResp, false, nil
		}

		if readResp.JSON200.OwnerId != expectedOwnerId {
			return readResp, false, nil
		}

		if readResp.JSON200.OwnerType != expectedOwnerType {
			return readResp, false, nil
		}

		if readResp.JSON200.ResourceType != expectedResourceType {
			return readResp, false, nil
		}

		if !permissionsMatch(readResp.JSON200.PermissionTypes, expectedPermissions) {
			return readResp, false, nil
		}

		if expectedVariant.isPropertyBased {
			actualResourcePropertyName := ""
			if readResp.JSON200.ResourcePropertyName != nil {
				actualResourcePropertyName = *readResp.JSON200.ResourcePropertyName
			}
			if actualResourcePropertyName != expectedVariant.resourcePropertyName {
				return readResp, false, nil
			}
			return readResp, true, nil
		}

		actualResourceId := ""
		if readResp.JSON200.ResourceId != nil {
			actualResourceId = *readResp.JSON200.ResourceId
		}
		if actualResourceId != expectedVariant.resourceId {
			return readResp, false, nil
		}

		return readResp, true, nil
	})
}

// permissionsMatch reports whether the permission types returned by the API are the same set
// as expected, ignoring order.
func permissionsMatch(actual []camunda.PermissionTypeEnum, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}

	actualSet := make(map[string]struct{}, len(actual))
	for _, p := range actual {
		actualSet[string(p)] = struct{}{}
	}

	for _, e := range expected {
		if _, ok := actualSet[e]; !ok {
			return false
		}
	}

	return true
}
