package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var _ resource.Resource = &RoleMemberClientResource{}
var _ resource.ResourceWithImportState = &RoleMemberClientResource{}

func NewRoleMemberClientResource() resource.Resource {
	return &RoleMemberClientResource{}
}

type RoleMemberClientResource struct {
	client *camunda.ClientWithResponses
}

type RoleMemberClientResourceModel struct {
	Id       types.String `tfsdk:"id"`
	RoleId   types.String `tfsdk:"role_id"`
	ClientId types.String `tfsdk:"client_id"`
}

func (r *RoleMemberClientResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role_member_client"
}

func (r *RoleMemberClientResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a client to a Camunda cluster role",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Composite ID of the assignment (`role_id/client_id`). Also used as the import ID: `terraform import camundacluster_role_member_client.example <role_id>/<client_id>`.",
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
			"client_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the client.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *RoleMemberClientResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RoleMemberClientResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RoleMemberClientResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.AssignRoleToClientWithResponse(ctx, data.RoleId.ValueString(), data.ClientId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to assign role to client, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusCreated && apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Assignment Error", fmt.Sprintf("Error while assigning role to client, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	data.Id = types.StringValue(data.RoleId.ValueString() + "/" + data.ClientId.ValueString())

	// Persist state from the plan plus the composite ID before polling for read
	// consistency, so a polling timeout or transport error doesn't orphan the assignment
	// the API already made.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err = waitForConsistency(ctx, fmt.Sprintf("role %q client %q assignment", data.RoleId.ValueString(), data.ClientId.ValueString()), func() (bool, bool, error) {
		found, err := searchAllRoleClients(ctx, r.client, data.RoleId.ValueString(), data.ClientId.ValueString())
		if err != nil {
			if err.Error() == "not_found" {
				// the role itself isn't found yet either — treat as pending, same reasoning
				return false, false, nil
			}
			return false, false, err
		}
		return found, found, nil
	})
	if err != nil {
		resp.Diagnostics.AddWarning("Consistency Check Failed", fmt.Sprintf("Client %q was assigned to role %q but could not be confirmed yet: %s. State was saved from the assignment response; a later refresh will pick up any drift.", data.ClientId.ValueString(), data.RoleId.ValueString(), err))
		return
	}

	tflog.Trace(ctx, "created role member client resource")
}

func (r *RoleMemberClientResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RoleMemberClientResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := searchAllRoleClients(ctx, r.client, data.RoleId.ValueString(), data.ClientId.ValueString())
	if err != nil {
		if err.Error() == "not_found" {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Error while reading role clients: %s", err))
		return
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// searchAllRoleClients pages through all role clients and returns true if clientId is found.
// Returns an error with message "not_found" if the role itself is not found.
func searchAllRoleClients(ctx context.Context, client *camunda.ClientWithResponses, roleId, clientId string) (bool, error) {
	var cursor string
	for {
		body := camunda.SearchClientsForRoleJSONRequestBody{}
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

		searchResp, err := client.SearchClientsForRoleWithResponse(ctx, roleId, body)
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
			Items []camunda.RoleClientResult `json:"items"`
			Page  struct {
				EndCursor         *string `json:"endCursor"`
				HasMoreTotalItems bool    `json:"hasMoreTotalItems"`
			} `json:"page"`
		}
		if err := json.Unmarshal(searchResp.Body, &page); err != nil {
			return false, fmt.Errorf("decode: %w", err)
		}

		for _, c := range page.Items {
			if c.ClientId == clientId {
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

func (r *RoleMemberClientResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected import ID in the format <role_id>/<client_id>, got: %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("client_id"), parts[1])...)
}

func (r *RoleMemberClientResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// No mutable state; all changes trigger replace via RequiresReplace
}

func (r *RoleMemberClientResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RoleMemberClientResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.UnassignRoleFromClientWithResponse(ctx, data.RoleId.ValueString(), data.ClientId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to unassign role from client, got error: %s", err))
		return
	}

	if apiResp.StatusCode() == http.StatusNotFound {
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Delete Error", fmt.Sprintf("Error while unassigning role from client, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	tflog.Trace(ctx, "deleted role member client resource")
}
