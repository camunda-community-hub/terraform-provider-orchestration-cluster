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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
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
	Permissions          types.Set    `tfsdk:"permissions"`
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
			"permissions": schema.SetAttribute{
				MarkdownDescription: "The permission types.",
				Required:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"resource_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the resource the permission relates to. Use \"*\" to match all resources.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"resource_property_name": schema.StringAttribute{
				MarkdownDescription: "The name of the resource property the permission relates to (mutually exclusive with " +
					"`resource_id`). This provider only creates and updates ID-based authorizations, so this attribute is " +
					"populated only when reading or importing a property-based authorization that was created outside of " +
					"this provider; it cannot be configured.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
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
	resp.Diagnostics.Append(data.Permissions.ElementsAs(ctx, &permissionStrings, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	permissionTypes := make([]camunda.PermissionTypeEnum, len(permissionStrings))
	for i, p := range permissionStrings {
		permissionTypes[i] = camunda.PermissionTypeEnum(p)
	}

	resourceId := "*"
	if !data.ResourceId.IsNull() && !data.ResourceId.IsUnknown() && data.ResourceId.ValueString() != "" {
		resourceId = data.ResourceId.ValueString()
	}

	idReq := camunda.AuthorizationIdBasedRequest{
		OwnerId:         data.OwnerId.ValueString(),
		OwnerType:       camunda.OwnerTypeEnum(data.OwnerType.ValueString()),
		PermissionTypes: permissionTypes,
		ResourceId:      resourceId,
		ResourceType:    camunda.ResourceTypeEnum(data.ResourceType.ValueString()),
	}

	var authReq camunda.AuthorizationRequest
	if err := authReq.FromAuthorizationIdBasedRequest(idReq); err != nil {
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

	// Re-read the full state from the engine, waiting out the eventual consistency of
	// GetAuthorization: an immediate read right after a successful create can still 404.
	getResp, err := readAuthorizationWithRetry(ctx, r.client, authKey)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read authorization after creation, got error: %s", err))
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
	resp.Diagnostics.Append(data.Permissions.ElementsAs(ctx, &permissionStrings, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	permissionTypes := make([]camunda.PermissionTypeEnum, len(permissionStrings))
	for i, p := range permissionStrings {
		permissionTypes[i] = camunda.PermissionTypeEnum(p)
	}

	resourceId := "*"
	if !data.ResourceId.IsNull() && !data.ResourceId.IsUnknown() && data.ResourceId.ValueString() != "" {
		resourceId = data.ResourceId.ValueString()
	}

	idReq := camunda.AuthorizationIdBasedRequest{
		OwnerId:         data.OwnerId.ValueString(),
		OwnerType:       camunda.OwnerTypeEnum(data.OwnerType.ValueString()),
		PermissionTypes: permissionTypes,
		ResourceId:      resourceId,
		ResourceType:    camunda.ResourceTypeEnum(data.ResourceType.ValueString()),
	}

	var updateReq camunda.UpdateAuthorizationJSONRequestBody
	if err := updateReq.FromAuthorizationIdBasedRequest(idReq); err != nil {
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
	getResp, err := readAuthorizationUntilConsistent(ctx, r.client, state.Id.ValueString(), permissionStrings, resourceId)
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

	if apiResp.StatusCode() != http.StatusNoContent {
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
	data.Permissions = permSet

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

// readAuthorizationUntilConsistent polls GET until it reflects the given permissions and
// resource ID, handling the same eventual consistency on updates as readAuthorizationWithRetry
// does on creates: the read-side projection can briefly return the pre-update values right
// after a successful update, which would otherwise make Terraform's post-apply refresh plan
// non-empty.
func readAuthorizationUntilConsistent(ctx context.Context, client *camunda.ClientWithResponses, authKey string, expectedPermissions []string, expectedResourceId string) (*camunda.GetAuthorizationResponse, error) {
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

		if !permissionsMatch(readResp.JSON200.PermissionTypes, expectedPermissions) {
			return readResp, false, nil
		}

		actualResourceId := ""
		if readResp.JSON200.ResourceId != nil {
			actualResourceId = *readResp.JSON200.ResourceId
		}
		if actualResourceId != expectedResourceId {
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
