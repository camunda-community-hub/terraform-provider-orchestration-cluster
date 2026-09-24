package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var _ resource.Resource = &GroupResource{}
var _ resource.ResourceWithImportState = &GroupResource{}

func NewGroupResource() resource.Resource {
	return &GroupResource{}
}

type GroupResource struct {
	client *camunda.ClientWithResponses
}

type GroupResourceModel struct {
	Id          types.String `tfsdk:"id"`
	GroupId     types.String `tfsdk:"group_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (r *GroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *GroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Camunda cluster group",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of the group (the group ID).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "The unique ID for the group.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The display name of the group.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the group.",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (r *GroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data GroupResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.CreateGroupJSONRequestBody{
		GroupId: data.GroupId.ValueString(),
		Name:    data.Name.ValueString(),
	}
	if !data.Description.IsNull() && !data.Description.IsUnknown() {
		desc := data.Description.ValueString()
		request.Description = &desc
	}

	apiResp, err := r.client.CreateGroupWithResponse(ctx, request)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create group, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusCreated {
		resp.Diagnostics.AddError("Not Created", fmt.Sprintf("Error while creating group, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 201 but with no parseable JSON body")
		return
	}

	if _, err := readGroupWithRetry(ctx, r.client, apiResp.JSON201.GroupId); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read group after creation, got error: %s", err))
		return
	}

	data.Id = types.StringValue(apiResp.JSON201.GroupId)
	data.GroupId = types.StringValue(apiResp.JSON201.GroupId)
	data.Name = types.StringValue(apiResp.JSON201.Name)
	if apiResp.JSON201.Description != nil {
		data.Description = types.StringValue(*apiResp.JSON201.Description)
	} else {
		data.Description = types.StringValue("")
	}

	tflog.Trace(ctx, "created group resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data GroupResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.GetGroupWithResponse(ctx, data.GroupId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read group '%s', got error: %s", data.GroupId.ValueString(), err))
		return
	}

	if apiResp.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Error while reading group, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	data.Id = types.StringValue(apiResp.JSON200.GroupId)
	data.GroupId = types.StringValue(apiResp.JSON200.GroupId)
	data.Name = types.StringValue(apiResp.JSON200.Name)
	if apiResp.JSON200.Description != nil {
		data.Description = types.StringValue(*apiResp.JSON200.Description)
	} else {
		data.Description = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data GroupResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state GroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.UpdateGroupJSONRequestBody{
		Name: data.Name.ValueString(),
	}
	if !data.Description.IsNull() && !data.Description.IsUnknown() {
		desc := data.Description.ValueString()
		request.Description = &desc
	}

	apiResp, err := r.client.UpdateGroupWithResponse(ctx, state.GroupId.ValueString(), request)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update group, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Not Updated", fmt.Sprintf("Error while updating group, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	// Re-read to confirm the update is reflected, waiting out the eventual consistency of
	// GetGroup: an immediate read right after a successful PUT can still return the old name
	// and overwrite the just-updated state.
	readResp, err := readGroupUntilConsistent(ctx, r.client, state.GroupId.ValueString(), data.Name.ValueString(), request.Description)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to confirm group update, got error: %s", err))
		return
	}

	if readResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	data.Id = types.StringValue(readResp.JSON200.GroupId)
	data.GroupId = types.StringValue(readResp.JSON200.GroupId)
	data.Name = types.StringValue(readResp.JSON200.Name)
	if readResp.JSON200.Description != nil {
		data.Description = types.StringValue(*readResp.JSON200.Description)
	} else {
		data.Description = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data GroupResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.DeleteGroupWithResponse(ctx, data.GroupId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete group, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting group, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}
}

func (r *GroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("group_id"), req, resp)
}

// readGroupWithRetry handles the eventual consistency of fetching a group by retrying a few
// times: if a group was just created, it may not be immediately available through the API.
func readGroupWithRetry(ctx context.Context, client *camunda.ClientWithResponses, groupId string) (*camunda.GetGroupResponse, error) {
	return waitForConsistency(ctx, fmt.Sprintf("group %q", groupId), func() (*camunda.GetGroupResponse, bool, error) {
		readResp, err := client.GetGroupWithResponse(ctx, groupId)
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

// readGroupUntilConsistent polls GET until it reflects the given name and description,
// handling the same eventual consistency on updates as readGroupWithRetry does on creates:
// the read-side projection can briefly return the pre-update values right after a successful
// PUT, which would otherwise make Terraform's post-apply refresh plan non-empty.
func readGroupUntilConsistent(ctx context.Context, client *camunda.ClientWithResponses, groupId, expectedName string, expectedDescription *string) (*camunda.GetGroupResponse, error) {
	return waitForConsistency(ctx, fmt.Sprintf("group %q", groupId), func() (*camunda.GetGroupResponse, bool, error) {
		readResp, err := client.GetGroupWithResponse(ctx, groupId)
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

		if readResp.JSON200.Name != expectedName {
			return readResp, false, nil
		}

		if normalizedDescription(readResp.JSON200.Description) != normalizedDescription(expectedDescription) {
			return readResp, false, nil
		}

		return readResp, true, nil
	})
}
