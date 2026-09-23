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
var _ resource.Resource = &ClientTenantAssignmentResource{}

func NewClientTenantAssignmentResource() resource.Resource {
	return &ClientTenantAssignmentResource{}
}

// ClientTenantAssignmentResource manages the membership of a client (an OAuth/OIDC
// client provisioned externally through the identity provider) in a Camunda cluster
// tenant. There is no independent lifecycle for a "client" object in the
// orchestration cluster API: this resource only models the assignment edge between an
// already existing client and a tenant.
type ClientTenantAssignmentResource struct {
	client *camunda.ClientWithResponses
}

// ClientTenantAssignmentResourceModel describes the resource data model.
type ClientTenantAssignmentResourceModel struct {
	Id       types.String `tfsdk:"id"`
	TenantId types.String `tfsdk:"tenant_id"`
	ClientId types.String `tfsdk:"client_id"`
}

func (r *ClientTenantAssignmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client_tenant_assignment"
}

func (r *ClientTenantAssignmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a client (an OAuth/OIDC client provisioned externally via the identity provider) to a Camunda cluster tenant.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of this assignment, composed of `tenant_id` and `client_id`.",
				Computed:            true,
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the tenant the client is assigned to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the client (as issued by the identity provider) to assign to the tenant.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *ClientTenantAssignmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ClientTenantAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClientTenantAssignmentResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tenantID := data.TenantId.ValueString()
	clientID := data.ClientId.ValueString()

	apiResp, err := r.client.AssignClientToTenantWithResponse(ctx, tenantID, clientID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to assign client '%s' to tenant '%s', got error: %s", clientID, tenantID, err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Not Created", fmt.Sprintf("Error while assigning client '%s' to tenant '%s', got HTTP error: %d: %s", clientID, tenantID, apiResp.StatusCode(), apiResp.Body))
		return
	}

	data.Id = types.StringValue(clientTenantAssignmentID(tenantID, clientID))

	tflog.Trace(ctx, "created client tenant assignment resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClientTenantAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClientTenantAssignmentResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tenantID := data.TenantId.ValueString()
	clientID := data.ClientId.ValueString()

	assigned, err := isClientAssignedToTenant(ctx, r.client, tenantID, clientID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read client tenant assignment for client '%s' in tenant '%s', got error: %s", clientID, tenantID, err))
		return
	}

	if !assigned {
		// The assignment no longer exists (e.g. it was removed out-of-band). Remove it
		// from state so Terraform plans to recreate it, rather than erroring.
		resp.State.RemoveResource(ctx)
		return
	}

	data.Id = types.StringValue(clientTenantAssignmentID(tenantID, clientID))

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClientTenantAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Both tenant_id and client_id are RequiresReplace, so there is nothing to
	// update in place: any change to either attribute forces a replacement and
	// this method is not expected to be called in practice. It is implemented
	// as a no-op that simply re-confirms the planned state, to satisfy the
	// resource.Resource interface.
	var data ClientTenantAssignmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	data.Id = types.StringValue(clientTenantAssignmentID(data.TenantId.ValueString(), data.ClientId.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClientTenantAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClientTenantAssignmentResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tenantID := data.TenantId.ValueString()
	clientID := data.ClientId.ValueString()

	apiResp, err := r.client.UnassignClientFromTenantWithResponse(ctx, tenantID, clientID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to unassign client '%s' from tenant '%s', got error: %s", clientID, tenantID, err))
		return
	}

	// A 404 means the tenant or the assignment is already gone, which is the desired
	// end state of a delete, so it is treated the same as a successful deletion.
	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while unassigning client '%s' from tenant '%s', got HTTP error: %d: %s", clientID, tenantID, apiResp.StatusCode(), apiResp.Body))
		return
	}
}

func clientTenantAssignmentID(tenantID, clientID string) string {
	return tenantID + "/" + clientID
}

// isClientAssignedToTenant checks whether clientID is currently assigned to tenantID
// by paging through the tenant's client search results.
//
// Note: the generated TenantClientSearchResult type (an alias of SearchQueryResponse)
// does not expose the "items" field that the API actually returns in the response
// body, and the search request body only supports pagination (no filter by client
// ID), so the response body is decoded manually here page by page instead of relying
// on the typed JSON200 field or a server-side filter.
func isClientAssignedToTenant(ctx context.Context, client *camunda.ClientWithResponses, tenantID, clientID string) (bool, error) {
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

		apiResp, err := client.SearchClientsForTenantWithResponse(ctx, tenantID, camunda.TenantClientSearchQueryRequest{
			Page: &pageRequest,
		})
		if err != nil {
			return false, fmt.Errorf("unable to search clients for tenant '%s': %w", tenantID, err)
		}

		if apiResp.StatusCode() != http.StatusOK {
			return false, fmt.Errorf("error while searching clients for tenant '%s', got HTTP error: %d: %s", tenantID, apiResp.StatusCode(), apiResp.Body)
		}

		var result struct {
			Items []struct {
				ClientId string `json:"clientId"`
			} `json:"items"`
		}
		if err := json.Unmarshal(apiResp.Body, &result); err != nil {
			return false, fmt.Errorf("unable to parse client search response for tenant '%s': %w", tenantID, err)
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

	return false, fmt.Errorf("exceeded maximum number of pages (%d) while searching clients for tenant '%s'", maxPages, tenantID)
}
