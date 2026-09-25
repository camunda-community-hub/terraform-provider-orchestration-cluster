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

var _ resource.Resource = &RoleResource{}
var _ resource.ResourceWithImportState = &RoleResource{}

func NewRoleResource() resource.Resource {
	return &RoleResource{}
}

type RoleResource struct {
	client *camunda.ClientWithResponses
}

type RoleResourceModel struct {
	Id          types.String `tfsdk:"id"`
	RoleId      types.String `tfsdk:"role_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (r *RoleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *RoleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Camunda cluster role",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of the role (the role ID).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"role_id": schema.StringAttribute{
				MarkdownDescription: "The unique ID for the role.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The display name of the role.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the role.",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (r *RoleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RoleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.CreateRoleJSONRequestBody{
		RoleId: data.RoleId.ValueString(),
		Name:   data.Name.ValueString(),
	}
	if !data.Description.IsNull() && !data.Description.IsUnknown() {
		desc := data.Description.ValueString()
		request.Description = &desc
	}

	apiResp, err := r.client.CreateRoleWithResponse(ctx, request)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create role, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusCreated {
		resp.Diagnostics.AddError("Not Created", fmt.Sprintf("Error while creating role, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 201 but with no parseable JSON body")
		return
	}

	if _, err := readRoleWithRetry(ctx, r.client, apiResp.JSON201.RoleId); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read role after creation, got error: %s", err))
		return
	}

	data.Id = types.StringValue(apiResp.JSON201.RoleId)
	data.RoleId = types.StringValue(apiResp.JSON201.RoleId)
	data.Name = types.StringValue(apiResp.JSON201.Name)
	if apiResp.JSON201.Description != nil {
		data.Description = types.StringValue(*apiResp.JSON201.Description)
	} else {
		data.Description = types.StringValue("")
	}

	tflog.Trace(ctx, "created role resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RoleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.GetRoleWithResponse(ctx, data.RoleId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read role '%s', got error: %s", data.RoleId.ValueString(), err))
		return
	}

	if apiResp.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Error while reading role '%s', got HTTP error: %d", data.RoleId.ValueString(), apiResp.StatusCode()))
		return
	}

	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	data.Id = types.StringValue(apiResp.JSON200.RoleId)
	data.RoleId = types.StringValue(apiResp.JSON200.RoleId)
	data.Name = types.StringValue(apiResp.JSON200.Name)
	if apiResp.JSON200.Description != nil {
		data.Description = types.StringValue(*apiResp.JSON200.Description)
	} else {
		data.Description = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RoleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state RoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.UpdateRoleJSONRequestBody{
		Name: data.Name.ValueString(),
	}
	if !data.Description.IsNull() && !data.Description.IsUnknown() {
		desc := data.Description.ValueString()
		request.Description = &desc
	}

	apiResp, err := r.client.UpdateRoleWithResponse(ctx, state.RoleId.ValueString(), request)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update role, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Not Updated", fmt.Sprintf("Error while updating role, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	// Re-read to confirm the update is reflected, waiting out the eventual consistency of
	// GetRole: an immediate read right after a successful PUT can still return the old name
	// and overwrite the just-updated state.
	readResp, err := readRoleUntilConsistent(ctx, r.client, state.RoleId.ValueString(), data.Name.ValueString(), request.Description)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to confirm role update, got error: %s", err))
		return
	}

	if readResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	data.Id = types.StringValue(readResp.JSON200.RoleId)
	data.RoleId = types.StringValue(readResp.JSON200.RoleId)
	data.Name = types.StringValue(readResp.JSON200.Name)
	if readResp.JSON200.Description != nil {
		data.Description = types.StringValue(*readResp.JSON200.Description)
	} else {
		data.Description = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RoleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.DeleteRoleWithResponse(ctx, data.RoleId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete role, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting role, got HTTP error: %d", apiResp.StatusCode()))
		return
	}
}

func (r *RoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("role_id"), req, resp)
}

// readRoleWithRetry handles the eventual consistency of fetching a role by retrying a few
// times: if a role was just created, it may not be immediately available through the API.
func readRoleWithRetry(ctx context.Context, client *camunda.ClientWithResponses, roleId string) (*camunda.GetRoleResponse, error) {
	return waitForConsistency(ctx, fmt.Sprintf("role %q", roleId), func() (*camunda.GetRoleResponse, bool, error) {
		readResp, err := client.GetRoleWithResponse(ctx, roleId)
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

// readRoleUntilConsistent polls GET until it reflects the given name and description,
// handling the same eventual consistency on updates as readRoleWithRetry does on creates:
// the read-side projection can briefly return the pre-update values right after a successful
// PUT, which would otherwise make Terraform's post-apply refresh plan non-empty.
func readRoleUntilConsistent(ctx context.Context, client *camunda.ClientWithResponses, roleId, expectedName string, expectedDescription *string) (*camunda.GetRoleResponse, error) {
	return waitForConsistency(ctx, fmt.Sprintf("role %q", roleId), func() (*camunda.GetRoleResponse, bool, error) {
		readResp, err := client.GetRoleWithResponse(ctx, roleId)
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
