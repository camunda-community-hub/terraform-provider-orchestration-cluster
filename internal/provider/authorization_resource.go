package provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

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
	Id           types.String `tfsdk:"id"`
	OwnerType    types.String `tfsdk:"owner_type"`
	OwnerId      types.String `tfsdk:"owner_id"`
	ResourceType types.String `tfsdk:"resource_type"`
	Permissions  types.Set    `tfsdk:"permissions"`
	ResourceId   types.String `tfsdk:"resource_id"`
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

	// Re-read the full state from the engine
	getResp, err := r.client.GetAuthorizationWithResponse(ctx, authKey)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read authorization after creation, got error: %s", err))
		return
	}

	if getResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Error while reading authorization after creation, got HTTP error: %d", getResp.StatusCode()))
		return
	}

	if getResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	data = authorizationResultToModel(data.Id, getResp.JSON200)

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

	data = authorizationResultToModel(data.Id, apiResp.JSON200)

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

	// Re-read to get current state
	getResp, err := r.client.GetAuthorizationWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read authorization after update, got error: %s", err))
		return
	}

	if getResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Error while reading authorization after update, got HTTP error: %d", getResp.StatusCode()))
		return
	}

	if getResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	data = authorizationResultToModel(state.Id, getResp.JSON200)

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

func authorizationResultToModel(id types.String, result *camunda.AuthorizationResult) AuthorizationResourceModel {
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
	permSet, _ := types.SetValueFrom(context.Background(), types.StringType, perms)
	data.Permissions = permSet

	if result.ResourceId != nil && *result.ResourceId != "" {
		data.ResourceId = types.StringValue(*result.ResourceId)
	} else {
		data.ResourceId = types.StringValue("*")
	}

	return data
}
