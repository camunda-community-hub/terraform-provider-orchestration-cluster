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
var _ resource.Resource = &ClientGroupAssignmentResource{}

func NewClientGroupAssignmentResource() resource.Resource {
	return &ClientGroupAssignmentResource{}
}

// ClientGroupAssignmentResource manages the membership of a client (an OAuth/OIDC
// client provisioned externally through the identity provider) in a Camunda cluster
// group. There is no independent lifecycle for a "client" object in the orchestration
// cluster API: this resource only models the assignment edge between an already
// existing client and a group.
type ClientGroupAssignmentResource struct {
	client *camunda.ClientWithResponses
}

// ClientGroupAssignmentResourceModel describes the resource data model.
type ClientGroupAssignmentResourceModel struct {
	Id       types.String `tfsdk:"id"`
	GroupId  types.String `tfsdk:"group_id"`
	ClientId types.String `tfsdk:"client_id"`
}

func (r *ClientGroupAssignmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client_group_assignment"
}

func (r *ClientGroupAssignmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a client (an OAuth/OIDC client provisioned externally via the identity provider) to a Camunda cluster group. Members of the group inherit the group's authorizations, roles, and tenant assignments.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of this assignment, composed of `group_id` and `client_id`.",
				Computed:            true,
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the group the client is assigned to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the client (as issued by the identity provider) to assign to the group.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *ClientGroupAssignmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ClientGroupAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClientGroupAssignmentResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	groupID := data.GroupId.ValueString()
	clientID := data.ClientId.ValueString()

	apiResp, err := r.client.AssignClientToGroupWithResponse(ctx, groupID, clientID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to assign client '%s' to group '%s', got error: %s", clientID, groupID, err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Not Created", fmt.Sprintf("Error while assigning client '%s' to group '%s', got HTTP error: %d: %s", clientID, groupID, apiResp.StatusCode(), apiResp.Body))
		return
	}

	data.Id = types.StringValue(clientGroupAssignmentID(groupID, clientID))

	tflog.Trace(ctx, "created client group assignment resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClientGroupAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClientGroupAssignmentResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	groupID := data.GroupId.ValueString()
	clientID := data.ClientId.ValueString()

	assigned, err := isClientAssignedToGroup(ctx, r.client, groupID, clientID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read client group assignment for client '%s' in group '%s', got error: %s", clientID, groupID, err))
		return
	}

	if !assigned {
		// The assignment no longer exists (e.g. it was removed out-of-band). Remove it
		// from state so Terraform plans to recreate it, rather than erroring.
		resp.State.RemoveResource(ctx)
		return
	}

	data.Id = types.StringValue(clientGroupAssignmentID(groupID, clientID))

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClientGroupAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Both group_id and client_id are RequiresReplace, so there is nothing to
	// update in place: any change to either attribute forces a replacement and
	// this method is not expected to be called in practice. It is implemented
	// as a no-op that simply re-confirms the planned state, to satisfy the
	// resource.Resource interface.
	var data ClientGroupAssignmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	data.Id = types.StringValue(clientGroupAssignmentID(data.GroupId.ValueString(), data.ClientId.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClientGroupAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClientGroupAssignmentResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	groupID := data.GroupId.ValueString()
	clientID := data.ClientId.ValueString()

	apiResp, err := r.client.UnassignClientFromGroupWithResponse(ctx, groupID, clientID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to unassign client '%s' from group '%s', got error: %s", clientID, groupID, err))
		return
	}

	// A 404 means the group or the assignment is already gone, which is the desired
	// end state of a delete, so it is treated the same as a successful deletion.
	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while unassigning client '%s' from group '%s', got HTTP error: %d: %s", clientID, groupID, apiResp.StatusCode(), apiResp.Body))
		return
	}
}

func clientGroupAssignmentID(groupID, clientID string) string {
	return groupID + "/" + clientID
}

// isClientAssignedToGroup checks whether clientID is currently assigned to groupID by
// paging through the group's client search results.
//
// Note: the generated GroupClientSearchResult type (an alias of SearchQueryResponse)
// does not expose the "items" field that the API actually returns in the response
// body, and the search request body only supports pagination (no filter by client
// ID), so the response body is decoded manually here page by page instead of relying
// on the typed JSON200 field or a server-side filter.
func isClientAssignedToGroup(ctx context.Context, client *camunda.ClientWithResponses, groupID, clientID string) (bool, error) {
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

		apiResp, err := client.SearchClientsForGroupWithResponse(ctx, groupID, camunda.GroupClientSearchQueryRequest{
			Page: &pageRequest,
		})
		if err != nil {
			return false, fmt.Errorf("unable to search clients for group '%s': %w", groupID, err)
		}

		if apiResp.StatusCode() != http.StatusOK {
			return false, fmt.Errorf("error while searching clients for group '%s', got HTTP error: %d: %s", groupID, apiResp.StatusCode(), apiResp.Body)
		}

		var result struct {
			Items []struct {
				ClientId string `json:"clientId"`
			} `json:"items"`
		}
		if err := json.Unmarshal(apiResp.Body, &result); err != nil {
			return false, fmt.Errorf("unable to parse client search response for group '%s': %w", groupID, err)
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

	return false, fmt.Errorf("exceeded maximum number of pages (%d) while searching clients for group '%s'", maxPages, groupID)
}
