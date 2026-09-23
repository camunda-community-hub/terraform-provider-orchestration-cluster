package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GroupResource{}
var _ resource.ResourceWithImportState = &GroupResource{}

func NewGroupResource() resource.Resource {
	return &GroupResource{}
}

// GroupResource defines the resource implementation.
type GroupResource struct {
	client *camunda.ClientWithResponses
}

// GroupResourceModel describes the resource data model.
type GroupResourceModel struct {
	Description types.String `tfsdk:"description"`
	GroupId     types.String `tfsdk:"group_id"`
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
}

func (r *GroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *GroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Camunda cluster group",

		Attributes: map[string]schema.Attribute{
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the group.",
				Optional:            true,
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of the group.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of a group (the group ID).",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The display name of the group.",
				Required:            true,
			},
		},
	}
}

func (r *GroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
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

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.CreateGroupJSONRequestBody{
		Description: data.Description.ValueStringPointer(),
		GroupId:     data.GroupId.ValueString(),
		Name:        data.Name.ValueString(),
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

	_, err = readGroupWithRetry(ctx, r.client, data.GroupId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read group after creation, got error: %s", err))
		return
	}

	data.Id = types.StringValue(apiResp.JSON201.GroupId)
	data.GroupId = types.StringValue(apiResp.JSON201.GroupId)
	data.Name = types.StringValue(apiResp.JSON201.Name)
	data.Description = optionalStringValue(apiResp.JSON201.Description)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created group resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data GroupResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := readGroupWithRetry(ctx, r.client, data.GroupId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read group '%s', got error: %s", data.GroupId.ValueString(), err))
		return
	}

	data.Id = types.StringValue(apiResp.JSON200.GroupId)
	data.GroupId = types.StringValue(apiResp.JSON200.GroupId)
	data.Name = types.StringValue(apiResp.JSON200.Name)
	data.Description = optionalStringValue(apiResp.JSON200.Description)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data GroupResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.UpdateGroupJSONRequestBody{
		Description: data.Description.ValueStringPointer(),
		Name:        data.Name.ValueString(),
	}
	apiResp, err := r.client.UpdateGroupWithResponse(ctx, data.GroupId.ValueString(), request)

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update group, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Not Updated", fmt.Sprintf("Error while updating group, got HTTP error: %d", apiResp.StatusCode()))
		return
	}

	data.Id = types.StringValue(apiResp.JSON200.GroupId)
	data.GroupId = types.StringValue(apiResp.JSON200.GroupId)
	data.Name = types.StringValue(apiResp.JSON200.Name)
	data.Description = optionalStringValue(apiResp.JSON200.Description)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data GroupResourceModel

	// Read Terraform prior state data into the model
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
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting group, got HTTP error: %d", apiResp.StatusCode()))
		return
	}
}

func (r *GroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("group_id"), req, resp)
}

// optionalStringValue converts a nullable API string pointer into a
// types.String, mapping a nil pointer to a null value instead of
// dereferencing it.
func optionalStringValue(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}

	return types.StringValue(*value)
}

// readGroupWithRetry handles the eventual consistency of fetching a group by retrying a few times.
func readGroupWithRetry(ctx context.Context, client *camunda.ClientWithResponses, groupId string) (*camunda.GetGroupResponse, error) {
	// Reading a group is eventually consistent: if a group is just created, it may not be immediately available through the API.
	// Handle the eventual consistency by retrying a few times with some delay in between until the group is found or we timeout.
	createState := &retry.StateChangeConf{
		Pending: []string{
			fmt.Sprintf("%d", http.StatusNotFound),
		},

		Target: []string{
			fmt.Sprintf("%d", http.StatusOK),
		},

		// How many times the target state has to be reached to continue.
		ContinuousTargetOccurence: 1,

		Refresh: func() (any, string, error) {
			readResp, err := client.GetGroupWithResponse(ctx, groupId)

			if err != nil {
				return nil, "", err
			}

			return readResp, fmt.Sprintf("%d", readResp.StatusCode()), nil
		},

		// Don't wait too long for the first poll
		Delay:      1 * time.Second,
		MinTimeout: 2 * time.Second,
		// Wait at most this duration before consideing the group has not been found
		Timeout: 30 * time.Second,
	}

	resp, err := createState.WaitForStateContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("timed out while waiting for group '%s' to be created: %w", groupId, err)
	}

	r, ok := resp.(*camunda.GetGroupResponse)
	if !ok {
		// This should not happen
		return nil, fmt.Errorf("unexpected type for group read response: %T", resp)
	}

	return r, nil
}
