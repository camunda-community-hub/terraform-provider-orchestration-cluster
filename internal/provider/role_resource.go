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
var _ resource.Resource = &RoleResource{}
var _ resource.ResourceWithImportState = &RoleResource{}

func NewRoleResource() resource.Resource {
	return &RoleResource{}
}

// RoleResource defines the resource implementation.
type RoleResource struct {
	client *camunda.ClientWithResponses
}

// RoleResourceModel describes the resource data model.
type RoleResourceModel struct {
	Description types.String `tfsdk:"description"`
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	RoleId      types.String `tfsdk:"role_id"`
}

func (r *RoleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *RoleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Camunda cluster role",

		Attributes: map[string]schema.Attribute{
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the role.",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of a role (the role ID).",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the role.",
				Required:            true,
			},
			"role_id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of a role.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *RoleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RoleResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.CreateRoleJSONRequestBody{
		RoleId:      data.RoleId.ValueString(),
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueStringPointer(),
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

	_, err = readRoleWithRetry(ctx, r.client, data.RoleId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read role after creation, got error: %s", err))
		return
	}

	data.Id = types.StringValue(apiResp.JSON201.RoleId)
	data.RoleId = types.StringValue(apiResp.JSON201.RoleId)
	data.Name = types.StringValue(apiResp.JSON201.Name)
	data.Description = optionalStringValue(apiResp.JSON201.Description)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created role resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RoleResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := readRoleWithRetry(ctx, r.client, data.RoleId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read role '%s', got error: %s", data.RoleId.ValueString(), err))
		return
	}

	data.Id = types.StringValue(apiResp.JSON200.RoleId)
	data.RoleId = types.StringValue(apiResp.JSON200.RoleId)
	data.Name = types.StringValue(apiResp.JSON200.Name)
	data.Description = optionalStringValue(apiResp.JSON200.Description)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RoleResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.UpdateRoleJSONRequestBody{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueStringPointer(),
	}
	apiResp, err := r.client.UpdateRoleWithResponse(ctx, data.RoleId.ValueString(), request)

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update role, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Not Updated", fmt.Sprintf("Error while updating role, got HTTP error: %d", apiResp.StatusCode()))
		return
	}

	data.Id = types.StringValue(data.RoleId.ValueString())
	data.Name = types.StringValue(apiResp.JSON200.Name)
	data.Description = optionalStringValue(apiResp.JSON200.Description)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RoleResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.DeleteRoleWithResponse(ctx, data.RoleId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete role, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting role, got HTTP error: %d", apiResp.StatusCode()))
		return
	}
}

func (r *RoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("role_id"), req, resp)
}

// optionalStringValue converts an optional (possibly nil) API string pointer into a
// types.String, mapping a nil pointer to a null value instead of dereferencing it.
func optionalStringValue(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}

	return types.StringValue(*value)
}

// readRoleWithRetry handles the eventual consistency of fetching a role by retrying a few times.
func readRoleWithRetry(ctx context.Context, client *camunda.ClientWithResponses, roleId string) (*camunda.GetRoleResponse, error) {
	// Reading a role is eventually consistent: if a role is just created, it may not be immediately available through the API.
	// Handle the eventual consistency by retrying a few times with some delay in between until the role is found or we timeout.
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
			readResp, err := client.GetRoleWithResponse(ctx, roleId)

			if err != nil {
				return nil, "", err
			}

			return readResp, fmt.Sprintf("%d", readResp.StatusCode()), nil
		},

		// Don't wait too long for the first poll
		Delay:      1 * time.Second,
		MinTimeout: 2 * time.Second,
		// Wait at most this duration before consideing the role has not been found
		Timeout: 30 * time.Second,
	}

	resp, err := createState.WaitForStateContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("timed out while waiting for role '%s' to be created: %w", roleId, err)
	}

	r, ok := resp.(*camunda.GetRoleResponse)
	if !ok {
		// This should not happen
		return nil, fmt.Errorf("unexpected type for role read response: %T", resp)
	}

	return r, nil
}
