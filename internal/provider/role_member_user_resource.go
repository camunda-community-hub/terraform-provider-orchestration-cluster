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

var _ resource.Resource = &RoleMemberUserResource{}

func NewRoleMemberUserResource() resource.Resource {
	return &RoleMemberUserResource{}
}

type RoleMemberUserResource struct {
	client *camunda.ClientWithResponses
}

type RoleMemberUserResourceModel struct {
	Id     types.String `tfsdk:"id"`
	RoleId types.String `tfsdk:"role_id"`
	UserId types.String `tfsdk:"user_id"`
}

func (r *RoleMemberUserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role_member_user"
}

func (r *RoleMemberUserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a user to a Camunda cluster role",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Composite ID of the assignment (role_id/user_id).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"role_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the role.",
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

func (r *RoleMemberUserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RoleMemberUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RoleMemberUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.AssignRoleToUserWithResponse(ctx, data.RoleId.ValueString(), data.UserId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to assign role to user, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusCreated && apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Assignment Error", fmt.Sprintf("Error while assigning role to user, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	data.Id = types.StringValue(data.RoleId.ValueString() + "/" + data.UserId.ValueString())

	tflog.Trace(ctx, "created role member user resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RoleMemberUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RoleMemberUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := searchAllRoleUsers(ctx, r.client, data.RoleId.ValueString(), data.UserId.ValueString())
	if err != nil {
		if err.Error() == "not_found" {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Error while reading role users: %s", err))
		return
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// searchAllRoleUsers pages through all role users and returns true if username is found.
// Returns an error with message "not_found" if the role itself is not found.
func searchAllRoleUsers(ctx context.Context, client *camunda.ClientWithResponses, roleId, username string) (bool, error) {
	var cursor string
	for {
		body := camunda.SearchUsersForRoleJSONRequestBody{}
		pageSize := int32(100)
		cursorPage := camunda.CursorForwardPagination{Limit: &pageSize}
		if cursor != "" {
			cursorPage.After = cursor
		}
		sqpr := camunda.SearchQueryPageRequest{}
		if err := sqpr.FromCursorForwardPagination(cursorPage); err != nil {
			return false, fmt.Errorf("encoding cursor: %w", err)
		}
		body.Page = &sqpr

		searchResp, err := client.SearchUsersForRoleWithResponse(ctx, roleId, body)
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
			Items []camunda.RoleUserResult `json:"items"`
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

func (r *RoleMemberUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// No mutable state; all changes trigger replace via RequiresReplace
}

func (r *RoleMemberUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RoleMemberUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.UnassignRoleFromUserWithResponse(ctx, data.RoleId.ValueString(), data.UserId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to unassign role from user, got error: %s", err))
		return
	}

	if apiResp.StatusCode() == http.StatusNotFound {
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Delete Error", fmt.Sprintf("Error while unassigning role from user, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	tflog.Trace(ctx, "deleted role member user resource")
}
