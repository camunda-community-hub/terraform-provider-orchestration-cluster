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
	"github.com/hashicorp/terraform-plugin-log/tflog"

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

	apiResp, err := r.client.AssignUserToGroupWithResponse(ctx, data.GroupId.ValueString(), data.UserId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to assign user to group, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusCreated && apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Assignment Error", fmt.Sprintf("Error while assigning user to group, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	_, err = waitForConsistency(ctx, fmt.Sprintf("group %q user %q assignment", data.GroupId.ValueString(), data.UserId.ValueString()), func() (bool, bool, error) {
		found, err := searchAllGroupUsers(ctx, r.client, data.GroupId.ValueString(), data.UserId.ValueString())
		if err != nil {
			if err.Error() == "not_found" {
				// the group itself isn't found yet either — treat as pending, same reasoning
				return false, false, nil
			}
			return false, false, err
		}
		return found, found, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to confirm user was assigned to group, got error: %s", err))
		return
	}

	data.Id = types.StringValue(data.GroupId.ValueString() + "/" + data.UserId.ValueString())

	tflog.Trace(ctx, "created group member user resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupMemberUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data GroupMemberUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := searchAllGroupUsers(ctx, r.client, data.GroupId.ValueString(), data.UserId.ValueString())
	if err != nil {
		if err.Error() == "not_found" {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Error while reading group users: %s", err))
		return
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// searchAllGroupUsers pages through all group users and returns true if username is found.
// Returns an error with message "not_found" if the group itself is not found.
func searchAllGroupUsers(ctx context.Context, client *camunda.ClientWithResponses, groupId, username string) (bool, error) {
	var cursor string
	for {
		body := camunda.SearchUsersForGroupJSONRequestBody{}
		pageSize := int32(100)
		// CursorForwardPagination.After has no `omitempty` json tag, so leaving it at its
		// zero value would still serialize an explicit `"after":""`, which the server
		// rejects as a malformed cursor (HTTP 500) on the very first page. Only set Page
		// at all once there is an actual cursor to send; the server applies its own
		// default paging behavior when the field is omitted entirely.
		if cursor != "" {
			cursorPage := camunda.CursorForwardPagination{Limit: &pageSize, After: cursor}
			sqpr := camunda.SearchQueryPageRequest{}
			if err := sqpr.FromCursorForwardPagination(cursorPage); err != nil {
				return false, fmt.Errorf("encoding cursor: %w", err)
			}
			body.Page = &sqpr
		}

		searchResp, err := client.SearchUsersForGroupWithResponse(ctx, groupId, body)
		if err != nil {
			return false, err
		}
		if searchResp.StatusCode() == http.StatusNotFound {
			return false, fmt.Errorf("not_found")
		}
		if searchResp.StatusCode() != http.StatusOK {
			return false, fmt.Errorf("HTTP %d", searchResp.StatusCode())
		}

		var page struct {
			Items []camunda.GroupUserResult `json:"items"`
			Page  struct {
				EndCursor         *string `json:"endCursor"`
				HasMoreTotalItems bool    `json:"hasMoreTotalItems"`
			} `json:"page"`
		}
		if err := json.Unmarshal(searchResp.Body, &page); err != nil {
			return false, fmt.Errorf("decode: %w", err)
		}

		for _, u := range page.Items {
			if u.Username == username {
				return true, nil
			}
		}

		if page.Page.EndCursor == nil || *page.Page.EndCursor == "" {
			break
		}
		cursor = *page.Page.EndCursor
	}
	return false, nil
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

	apiResp, err := r.client.UnassignUserFromGroupWithResponse(ctx, data.GroupId.ValueString(), data.UserId.ValueString())
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

	tflog.Trace(ctx, "deleted group member user resource")
}
