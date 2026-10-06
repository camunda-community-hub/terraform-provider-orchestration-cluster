package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

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
				MarkdownDescription: descriptionAttributeMarkdown("tenant"),
				Optional:            true,
				Validators: []validator.String{
					nonEmptyStringValidator{},
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of a tenant (the tenant ID).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the tenant.",
				Required:            true,
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "The unique ID for the tenant. Must be 31 characters or less. Can contain letters, numbers, `_`, `-`, `.`. " +
					"Note: the tenant-creation REST API itself accepts a looser format (up to 256 characters, also allowing `+` and `@`), " +
					"but a tenant ID actually used to scope process orchestration operations (starting process instances, publishing " +
					"messages, broadcasting signals, evaluating decisions) is validated by the Zeebe gateway against this stricter rule. " +
					"A tenant created outside these bounds would exist but be unusable for orchestration, so this provider enforces the " +
					"stricter, practically-usable format at creation time.",
				Required: true,
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

// tenantIdValidator validates that a tenant ID is 31 characters or less and only
// contains letters, numbers, `_`, `-` and `.`. This is stricter than the tenant-creation
// REST API's own contract (256 characters, also allowing `+` and `@`), matching instead
// the Zeebe gateway's tenant ID validation used for process orchestration operations
// (starting process instances, publishing messages, broadcasting signals, evaluating
// decisions). A tenant created with an ID outside this stricter format would be created
// successfully but could not actually be used to scope those operations.
type tenantIdValidator struct{}

var tenantIdPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,31}$`)

func (v tenantIdValidator) Description(ctx context.Context) string {
	return "must be 31 characters or less and contain only letters, numbers, '_', '-' and '.'"
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
			fmt.Sprintf("tenant_id %q must be 31 characters or less and contain only letters, numbers, '_', '-' and '.'.", req.ConfigValue.ValueString()),
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

	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 201 but with no parseable JSON body")
		return
	}

	data.Id = types.StringValue(apiResp.JSON201.TenantId)
	data.TenantId = types.StringValue(apiResp.JSON201.TenantId)
	data.Name = types.StringValue(apiResp.JSON201.Name)
	data.Description = optionalStringValue(apiResp.JSON201.Description)

	// Persist state from the create response before polling for read consistency, so a
	// polling timeout or transport error doesn't orphan the tenant the API already created.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := readTenantWithRetry(ctx, r.client, apiResp.JSON201.TenantId); err != nil {
		resp.Diagnostics.AddWarning("Consistency Check Failed", fmt.Sprintf("Tenant %q was created but could not be confirmed readable yet: %s. State was saved from the create response; a later refresh will pick up any drift.", apiResp.JSON201.TenantId, err))
		return
	}

	tflog.Trace(ctx, "created tenant resource")
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

	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
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

	readResp, err := readTenantUntilConsistent(ctx, r.client, data.TenantId.ValueString(), data.Name.ValueString(), data.Description.ValueStringPointer())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to confirm tenant update, got error: %s", err))
		return
	}

	if readResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	data.Id = types.StringValue(readResp.JSON200.TenantId)
	data.TenantId = types.StringValue(readResp.JSON200.TenantId)
	data.Name = types.StringValue(readResp.JSON200.Name)
	data.Description = optionalStringValue(readResp.JSON200.Description)

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

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting tenant, got HTTP error: %d", apiResp.StatusCode()))
		return
	}
}

func (r *TenantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("tenant_id"), req, resp)
}

// readTenantWithRetry handles the eventual consistency of fetching a tenant by retrying a few times:
// if a tenant was just created, it may not be immediately available through the API.
func readTenantWithRetry(ctx context.Context, client *camunda.ClientWithResponses, tenantId string) (*camunda.GetTenantResponse, error) {
	return readWithRetry(ctx, client, fmt.Sprintf("tenant %q", tenantId),
		func() (*camunda.GetTenantResponse, error) {
			return client.GetTenantWithResponse(ctx, tenantId)
		},
		func(readResp *camunda.GetTenantResponse) (bool, error) {
			switch readResp.StatusCode() {
			case http.StatusOK:
				return true, nil
			case http.StatusNotFound:
				return false, nil
			default:
				return false, fmt.Errorf("got HTTP error: %d: %s", readResp.StatusCode(), readResp.Body)
			}
		})
}

// readTenantUntilConsistent polls GET until it reflects the given name and description,
// handling the same eventual consistency on updates as readTenantWithRetry does on creates:
// the read-side projection can briefly return the pre-update values right after a successful
// PUT, which would otherwise make Terraform's post-apply refresh plan non-empty.
func readTenantUntilConsistent(ctx context.Context, client *camunda.ClientWithResponses, tenantId, expectedName string, expectedDescription *string) (*camunda.GetTenantResponse, error) {
	return readUntilConsistent(ctx, client, fmt.Sprintf("tenant %q", tenantId),
		func() (*camunda.GetTenantResponse, error) {
			return client.GetTenantWithResponse(ctx, tenantId)
		},
		func(readResp *camunda.GetTenantResponse) (bool, error) {
			if readResp.StatusCode() == http.StatusNotFound {
				return false, nil
			}

			if readResp.StatusCode() != http.StatusOK {
				return false, fmt.Errorf("got HTTP error: %d: %s", readResp.StatusCode(), readResp.Body)
			}

			if readResp.JSON200 == nil {
				return false, nil
			}

			if readResp.JSON200.Name != expectedName {
				return false, nil
			}

			if normalizedDescription(readResp.JSON200.Description) != normalizedDescription(expectedDescription) {
				return false, nil
			}

			return true, nil
		})
}
