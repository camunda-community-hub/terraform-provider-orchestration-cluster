package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var _ resource.Resource = &GroupMemberUserResource{}

func NewGroupMemberUserResource() resource.Resource {
	return &GroupMemberUserResource{}
}

type GroupMemberUserResource struct {
	client *camunda.ClientWithResponses
}

type GroupMemberUserResourceModel struct {
	Id      types.String `tfsdk:"id"`
	GroupId types.String `tfsdk:"group_id"`
	UserId  types.String `tfsdk:"user_id"`
}

func (r *GroupMemberUserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_member_user"
}

func (r *GroupMemberUserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a user to a Camunda cluster group",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Composite ID of the assignment (group_id/user_id).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the group.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_id": schema.StringAttribute{
				MarkdownDescription: "The username of the user.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *GroupMemberUserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GroupMemberUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data GroupMemberUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.AssignUserToGroupWithResponse(ctx, data.GroupId.ValueString(), camunda.Username(data.UserId.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to assign user to group, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusCreated && apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Assignment Error", fmt.Sprintf("Error while assigning user to group, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	data.Id = types.StringValue(data.GroupId.ValueString() + "/" + data.UserId.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupMemberUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data GroupMemberUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	searchResp, err := r.client.SearchUsersForGroupWithResponse(ctx, data.GroupId.ValueString(), camunda.SearchUsersForGroupJSONRequestBody{})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to search users for group, got error: %s", err))
		return
	}

	if searchResp.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if searchResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Error while reading group users, got HTTP error: %d", searchResp.StatusCode()))
		return
	}

	// Decode items from the raw body
	var rawResult struct {
		Items []camunda.GroupUserResult `json:"items"`
	}
	if err := json.Unmarshal(searchResp.Body, &rawResult); err != nil {
		resp.Diagnostics.AddError("Decode Error", fmt.Sprintf("Unable to decode search response: %s", err))
		return
	}

	found := false
	for _, u := range rawResult.Items {
		if string(u.Username) == data.UserId.ValueString() {
			found = true
			break
		}
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupMemberUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// No mutable state; all changes trigger replace via RequiresReplace
}

func (r *GroupMemberUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data GroupMemberUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.UnassignUserFromGroupWithResponse(ctx, data.GroupId.ValueString(), camunda.Username(data.UserId.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to unassign user from group, got error: %s", err))
		return
	}

	if apiResp.StatusCode() == http.StatusNotFound {
		// Already gone
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Delete Error", fmt.Sprintf("Error while unassigning user from group, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}
}
