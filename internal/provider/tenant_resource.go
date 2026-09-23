package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &TenantResource{}
var _ resource.ResourceWithImportState = &TenantResource{}

func NewTenantResource() resource.Resource {
	return &TenantResource{}
}

// TenantResource defines the resource implementation.
type TenantResource struct {
	client *camunda.ClientWithResponses
}

// TenantResourceModel describes the resource data model.
type TenantResourceModel struct {
	Description types.String `tfsdk:"description"`
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	TenantId    types.String `tfsdk:"tenant_id"`
}

func (r *TenantResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant"
}

func (r *TenantResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Camunda cluster tenant",

		Attributes: map[string]schema.Attribute{
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the tenant.",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of a tenant (the tenant ID).",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the tenant.",
				Required:            true,
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "The unique ID for the tenant. Must be 255 characters or less. Can contain letters, numbers, `_`, `-`, `+`, `.`, `@`.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					tenantIdValidator{},
				},
			},
		},
	}
}

// tenantIdValidator validates that a tenant ID is 255 characters or less and only
// contains letters, numbers, `_`, `-`, `+`, `.` and `@`, as required by the Camunda
// cluster REST API.
type tenantIdValidator struct{}

var tenantIdPattern = regexp.MustCompile(`^[A-Za-z0-9_\-+.@]{1,255}$`)

func (v tenantIdValidator) Description(ctx context.Context) string {
	return "must be 255 characters or less and contain only letters, numbers, '_', '-', '+', '.' and '@'"
}

func (v tenantIdValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v tenantIdValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if !tenantIdPattern.MatchString(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Tenant ID",
			fmt.Sprintf("tenant_id %q must be 255 characters or less and contain only letters, numbers, '_', '-', '+', '.' and '@'.", req.ConfigValue.ValueString()),
		)
	}
}

func (r *TenantResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TenantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TenantResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.CreateTenantJSONRequestBody{
		Description: data.Description.ValueStringPointer(),
		Name:        data.Name.ValueString(),
		TenantId:    data.TenantId.ValueString(),
	}
	apiResp, err := r.client.CreateTenantWithResponse(ctx, request)

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create tenant, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusCreated {
		resp.Diagnostics.AddError("Not Created", fmt.Sprintf("Error while creating tenant, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	_, err = readTenantWithRetry(ctx, r.client, data.TenantId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read tenant after creation, got error: %s", err))
		return
	}

	data.Id = types.StringValue(apiResp.JSON201.TenantId)
	data.TenantId = types.StringValue(apiResp.JSON201.TenantId)
	data.Name = types.StringValue(apiResp.JSON201.Name)
	data.Description = optionalStringValue(apiResp.JSON201.Description)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created tenant resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TenantResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.GetTenantWithResponse(ctx, data.TenantId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read tenant '%s', got error: %s", data.TenantId.ValueString(), err))
		return
	}

	if apiResp.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read tenant '%s', got HTTP error: %d: %s", data.TenantId.ValueString(), apiResp.StatusCode(), apiResp.Body))
		return
	}

	data.Id = types.StringValue(apiResp.JSON200.TenantId)
	data.TenantId = types.StringValue(apiResp.JSON200.TenantId)
	data.Name = types.StringValue(apiResp.JSON200.Name)
	data.Description = optionalStringValue(apiResp.JSON200.Description)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data TenantResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.UpdateTenantJSONRequestBody{
		Description: data.Description.ValueStringPointer(),
		Name:        data.Name.ValueString(),
	}
	apiResp, err := r.client.UpdateTenantWithResponse(ctx, data.TenantId.ValueString(), request)

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update tenant, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Not Updated", fmt.Sprintf("Error while updating tenant, got HTTP error: %d", apiResp.StatusCode()))
		return
	}

	data.Id = types.StringValue(apiResp.JSON200.TenantId)
	data.TenantId = types.StringValue(apiResp.JSON200.TenantId)
	data.Name = types.StringValue(apiResp.JSON200.Name)
	data.Description = optionalStringValue(apiResp.JSON200.Description)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TenantResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.DeleteTenantWithResponse(ctx, data.TenantId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete tenant, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting tenant, got HTTP error: %d", apiResp.StatusCode()))
		return
	}
}

func (r *TenantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("tenant_id"), req, resp)
}

// optionalStringValue converts an optional *string, as returned by the API for the
// tenant description, into a types.String, mapping a nil pointer to a null value.
func optionalStringValue(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// readTenantWithRetry handles the eventual consistency of fetching a tenant by retrying a few times.
func readTenantWithRetry(ctx context.Context, client *camunda.ClientWithResponses, tenantId string) (*camunda.GetTenantResponse, error) {
	// Reading a tenant is eventually consistent: if a tenant is just created, it may not be immediately available through the API.
	// Handle the eventual consistency by retrying a few times with some delay in between until the tenant is found or we timeout.
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
			readResp, err := client.GetTenantWithResponse(ctx, tenantId)

			if err != nil {
				return nil, "", err
			}

			return readResp, fmt.Sprintf("%d", readResp.StatusCode()), nil
		},

		// Don't wait too long for the first poll
		Delay:      1 * time.Second,
		MinTimeout: 2 * time.Second,
		// Wait at most this duration before considering the tenant has not been found
		Timeout: 30 * time.Second,
	}

	resp, err := createState.WaitForStateContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("timed out while waiting for tenant '%s' to be created: %w", tenantId, err)
	}

	r, ok := resp.(*camunda.GetTenantResponse)
	if !ok {
		// This should not happen
		return nil, fmt.Errorf("unexpected type for tenant read response: %T", resp)
	}

	return r, nil
}
