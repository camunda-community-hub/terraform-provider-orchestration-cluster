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

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ClientRoleAssignmentResource{}

func NewClientRoleAssignmentResource() resource.Resource {
	return &ClientRoleAssignmentResource{}
}

// ClientRoleAssignmentResource manages the assignment of a role to a client (an
// OAuth/OIDC client provisioned externally through the identity provider). There is
// no independent lifecycle for a "client" object in the orchestration cluster API:
// this resource only models the assignment edge between an already existing client
// and a role.
//
// Note: the underlying API operations are named "assignRoleToClient" /
// "unassignRoleFromClient" (reversed compared to the group/tenant equivalents), but
// the path and parameter order are still (roleId, clientId).
type ClientRoleAssignmentResource struct {
	client *camunda.ClientWithResponses
}

// ClientRoleAssignmentResourceModel describes the resource data model.
type ClientRoleAssignmentResourceModel struct {
	Id       types.String `tfsdk:"id"`
	RoleId   types.String `tfsdk:"role_id"`
	ClientId types.String `tfsdk:"client_id"`
}

func (r *ClientRoleAssignmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client_role_assignment"
}

func (r *ClientRoleAssignmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a role to a client (an OAuth/OIDC client provisioned externally via the identity provider). The client inherits the authorizations associated with the role.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of this assignment, composed of `role_id` and `client_id`.",
				Computed:            true,
			},
			"role_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the role assigned to the client.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the client (as issued by the identity provider) the role is assigned to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *ClientRoleAssignmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ClientRoleAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClientRoleAssignmentResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	roleID := data.RoleId.ValueString()
	clientID := data.ClientId.ValueString()

	apiResp, err := r.client.AssignRoleToClientWithResponse(ctx, roleID, clientID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to assign role '%s' to client '%s', got error: %s", roleID, clientID, err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Not Created", fmt.Sprintf("Error while assigning role '%s' to client '%s', got HTTP error: %d: %s", roleID, clientID, apiResp.StatusCode(), apiResp.Body))
		return
	}

	data.Id = types.StringValue(clientRoleAssignmentID(roleID, clientID))

	tflog.Trace(ctx, "created client role assignment resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClientRoleAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClientRoleAssignmentResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	roleID := data.RoleId.ValueString()
	clientID := data.ClientId.ValueString()

	assigned, err := isRoleAssignedToClient(ctx, r.client, roleID, clientID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read client role assignment for role '%s' and client '%s', got error: %s", roleID, clientID, err))
		return
	}

	if !assigned {
		// The assignment no longer exists (e.g. it was removed out-of-band). Remove it
		// from state so Terraform plans to recreate it, rather than erroring.
		resp.State.RemoveResource(ctx)
		return
	}

	data.Id = types.StringValue(clientRoleAssignmentID(roleID, clientID))

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClientRoleAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Both role_id and client_id are RequiresReplace, so there is nothing to
	// update in place: any change to either attribute forces a replacement and
	// this method is not expected to be called in practice. It is implemented
	// as a no-op that simply re-confirms the planned state, to satisfy the
	// resource.Resource interface.
	var data ClientRoleAssignmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	data.Id = types.StringValue(clientRoleAssignmentID(data.RoleId.ValueString(), data.ClientId.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClientRoleAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClientRoleAssignmentResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	roleID := data.RoleId.ValueString()
	clientID := data.ClientId.ValueString()

	apiResp, err := r.client.UnassignRoleFromClientWithResponse(ctx, roleID, clientID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to unassign role '%s' from client '%s', got error: %s", roleID, clientID, err))
		return
	}

	// A 404 means the role, the client, or the assignment is already gone, which is
	// the desired end state of a delete, so it is treated the same as a successful
	// deletion.
	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while unassigning role '%s' from client '%s', got HTTP error: %d: %s", roleID, clientID, apiResp.StatusCode(), apiResp.Body))
		return
	}
}

func clientRoleAssignmentID(roleID, clientID string) string {
	return roleID + "/" + clientID
}

// isRoleAssignedToClient checks whether roleID is currently assigned to clientID by
// paging through the role's client search results.
//
// Note: the generated RoleClientSearchResult type (an alias of SearchQueryResponse)
// does not expose the "items" field that the API actually returns in the response
// body, and the search request body only supports pagination (no filter by client
// ID), so the response body is decoded manually here page by page instead of relying
// on the typed JSON200 field or a server-side filter.
func isRoleAssignedToClient(ctx context.Context, client *camunda.ClientWithResponses, roleID, clientID string) (bool, error) {
	const pageSize = int32(100)
	const maxPages = 1000

	from := int32(0)

	for page := 0; page < maxPages; page++ {
		limit := pageSize
		pageRequest := camunda.SearchQueryPageRequest{}
		if err := pageRequest.FromOffsetPagination(camunda.OffsetPagination{
			From:  &from,
			Limit: &limit,
		}); err != nil {
			return false, fmt.Errorf("unable to build search page request: %w", err)
		}

		apiResp, err := client.SearchClientsForRoleWithResponse(ctx, roleID, camunda.RoleClientSearchQueryRequest{
			Page: &pageRequest,
		})
		if err != nil {
			return false, fmt.Errorf("unable to search clients for role '%s': %w", roleID, err)
		}

		if apiResp.StatusCode() != http.StatusOK {
			return false, fmt.Errorf("error while searching clients for role '%s', got HTTP error: %d: %s", roleID, apiResp.StatusCode(), apiResp.Body)
		}

		var result struct {
			Items []struct {
				ClientId string `json:"clientId"`
			} `json:"items"`
		}
		if err := json.Unmarshal(apiResp.Body, &result); err != nil {
			return false, fmt.Errorf("unable to parse client search response for role '%s': %w", roleID, err)
		}

		for _, item := range result.Items {
			if item.ClientId == clientID {
				return true, nil
			}
		}

		if int32(len(result.Items)) < limit {
			// Last page reached without a match.
			return false, nil
		}

		from += limit
	}

	return false, fmt.Errorf("exceeded maximum number of pages (%d) while searching clients for role '%s'", maxPages, roleID)
}
