package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
var _ resource.Resource = &AuthorizationResource{}
var _ resource.ResourceWithImportState = &AuthorizationResource{}

func NewAuthorizationResource() resource.Resource {
	return &AuthorizationResource{}
}

// AuthorizationResource defines the resource implementation.
type AuthorizationResource struct {
	client *camunda.ClientWithResponses
}

// AuthorizationResourceModel describes the resource data model.
//
// The underlying API models an authorization as a discriminated union: either "ID-based"
// (resource_id set) or "property-based" (resource_property_name set). Rather than mirroring
// that union with nested blocks, both fields are exposed as flat optional attributes and
// mutual exclusivity is enforced in Create/Update, since it is just two scalar strings.
type AuthorizationResourceModel struct {
	AuthorizationKey     types.String `tfsdk:"authorization_key"`
	OwnerId              types.String `tfsdk:"owner_id"`
	OwnerType            types.String `tfsdk:"owner_type"`
	PermissionTypes      types.List   `tfsdk:"permission_types"`
	ResourceType         types.String `tfsdk:"resource_type"`
	ResourceId           types.String `tfsdk:"resource_id"`
	ResourcePropertyName types.String `tfsdk:"resource_property_name"`
}

func (r *AuthorizationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_authorization"
}

func (r *AuthorizationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Camunda cluster authorization, granting a set of permissions on a resource (or resource property) to an owner such as a user, group, role, client, or mapping rule.\n\n" +
			"Exactly one of `resource_id` or `resource_property_name` must be set.",

		Attributes: map[string]schema.Attribute{
			"authorization_key": schema.StringAttribute{
				MarkdownDescription: "The server-assigned unique key of the authorization. There is no user-supplied ID for this resource; this value is populated after creation and used to address the resource for read, update, delete, and import.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"owner_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the owner of the permissions (for example a username, group ID, role ID, client ID, or mapping rule ID, depending on `owner_type`).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"owner_type": schema.StringAttribute{
				MarkdownDescription: "The type of the owner of the permissions, e.g. `USER`, `GROUP`, `ROLE`, `CLIENT`, `MAPPING_RULE`, or `UNSPECIFIED`. See the Camunda REST API documentation for the authoritative list of values.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"permission_types": schema.ListAttribute{
				MarkdownDescription: "The permission types granted by this authorization, e.g. `READ`, `CREATE`, `UPDATE`, `DELETE`. See the Camunda REST API documentation for the valid permission types per resource type. This is the only attribute that can be changed in place; all other attributes require replacing the resource.",
				Required:            true,
				ElementType:         types.StringType,
			},
			"resource_type": schema.StringAttribute{
				MarkdownDescription: "The type of resource the permissions relate to, e.g. `PROCESS_DEFINITION`, `USER`, `GROUP`. See the Camunda REST API documentation for the authoritative list of values.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the resource the permissions relate to. Mutually exclusive with `resource_property_name`; exactly one of the two must be set.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource_property_name": schema.StringAttribute{
				MarkdownDescription: "The name of the resource property the permissions relate to. Mutually exclusive with `resource_id`; exactly one of the two must be set.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *AuthorizationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AuthorizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AuthorizationResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	request, diags := authorizationRequestFromModel(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.CreateAuthorizationWithResponse(ctx, request)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create authorization, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusCreated {
		resp.Diagnostics.AddError("Not Created", fmt.Sprintf("Error while creating authorization, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	authorizationKey := apiResp.JSON201.AuthorizationKey

	// The create endpoint only returns the generated authorization key, not the full
	// resource. Fetch the full resource via Get (with retry, since Get is documented as
	// eventually consistent) to populate the remaining computed state.
	readResp, err := readAuthorizationWithRetry(ctx, r.client, authorizationKey)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read authorization after creation, got error: %s", err))
		return
	}

	resp.Diagnostics.Append(applyAuthorizationResult(ctx, &data, readResp.JSON200)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created authorization resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuthorizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AuthorizationResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := readAuthorizationWithRetry(ctx, r.client, data.AuthorizationKey.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read authorization '%s', got error: %s", data.AuthorizationKey.ValueString(), err))
		return
	}

	resp.Diagnostics.Append(applyAuthorizationResult(ctx, &data, apiResp.JSON200)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuthorizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AuthorizationResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// owner_id, owner_type, resource_type, resource_id, and resource_property_name are all
	// RequiresReplace, so only permission_types can reach Update in practice. The update
	// endpoint nonetheless takes the same union body as create, so it is rebuilt in full.
	request, diags := authorizationRequestFromModel(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	authorizationKey := data.AuthorizationKey.ValueString()

	apiResp, err := r.client.UpdateAuthorizationWithResponse(ctx, authorizationKey, request)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update authorization, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Not Updated", fmt.Sprintf("Error while updating authorization, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	// Update returns no body, so re-read the resource to refresh computed state.
	readResp, err := readAuthorizationWithRetry(ctx, r.client, authorizationKey)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read authorization after update, got error: %s", err))
		return
	}

	resp.Diagnostics.Append(applyAuthorizationResult(ctx, &data, readResp.JSON200)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuthorizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AuthorizationResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.DeleteAuthorizationWithResponse(ctx, data.AuthorizationKey.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete authorization, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting authorization, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}
}

func (r *AuthorizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("authorization_key"), req, resp)
}

// authorizationRequestFromModel validates the resource_id / resource_property_name mutual
// exclusivity and builds the union request body (AuthorizationRequest) shared by Create and
// Update.
func authorizationRequestFromModel(ctx context.Context, data AuthorizationResourceModel) (camunda.AuthorizationRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	var request camunda.AuthorizationRequest

	hasResourceId := !data.ResourceId.IsNull() && !data.ResourceId.IsUnknown() && data.ResourceId.ValueString() != ""
	hasResourcePropertyName := !data.ResourcePropertyName.IsNull() && !data.ResourcePropertyName.IsUnknown() && data.ResourcePropertyName.ValueString() != ""

	if hasResourceId == hasResourcePropertyName {
		diags.AddAttributeError(
			path.Root("resource_id"),
			"Invalid Attribute Combination",
			"Exactly one of \"resource_id\" or \"resource_property_name\" must be set, not both or neither.",
		)
		return request, diags
	}

	var permissionTypeValues []string
	diags.Append(data.PermissionTypes.ElementsAs(ctx, &permissionTypeValues, false)...)
	if diags.HasError() {
		return request, diags
	}

	permissionTypes := make([]camunda.PermissionTypeEnum, len(permissionTypeValues))
	for i, v := range permissionTypeValues {
		permissionTypes[i] = camunda.PermissionTypeEnum(v)
	}

	var err error
	if hasResourceId {
		err = request.FromAuthorizationIdBasedRequest(camunda.AuthorizationIdBasedRequest{
			OwnerId:         data.OwnerId.ValueString(),
			OwnerType:       camunda.OwnerTypeEnum(data.OwnerType.ValueString()),
			PermissionTypes: permissionTypes,
			ResourceId:      data.ResourceId.ValueString(),
			ResourceType:    camunda.ResourceTypeEnum(data.ResourceType.ValueString()),
		})
	} else {
		err = request.FromAuthorizationPropertyBasedRequest(camunda.AuthorizationPropertyBasedRequest{
			OwnerId:              data.OwnerId.ValueString(),
			OwnerType:            camunda.OwnerTypeEnum(data.OwnerType.ValueString()),
			PermissionTypes:      permissionTypes,
			ResourcePropertyName: data.ResourcePropertyName.ValueString(),
			ResourceType:         camunda.ResourceTypeEnum(data.ResourceType.ValueString()),
		})
	}

	if err != nil {
		diags.AddError("Client Error", fmt.Sprintf("Unable to build authorization request body: %s", err))
	}

	return request, diags
}

// applyAuthorizationResult copies the fields of an AuthorizationResult returned by the API
// into the resource model.
func applyAuthorizationResult(ctx context.Context, data *AuthorizationResourceModel, result *camunda.AuthorizationResult) diag.Diagnostics {
	var diags diag.Diagnostics

	data.AuthorizationKey = types.StringValue(result.AuthorizationKey)
	data.OwnerId = types.StringValue(result.OwnerId)
	data.OwnerType = types.StringValue(string(result.OwnerType))
	data.ResourceType = types.StringValue(string(result.ResourceType))

	if result.ResourceId != nil {
		data.ResourceId = types.StringValue(*result.ResourceId)
	} else {
		data.ResourceId = types.StringNull()
	}

	if result.ResourcePropertyName != nil {
		data.ResourcePropertyName = types.StringValue(*result.ResourcePropertyName)
	} else {
		data.ResourcePropertyName = types.StringNull()
	}

	permissionTypeValues := make([]string, len(result.PermissionTypes))
	for i, pt := range result.PermissionTypes {
		permissionTypeValues[i] = string(pt)
	}

	permissionTypesList, listDiags := types.ListValueFrom(ctx, types.StringType, permissionTypeValues)
	diags.Append(listDiags...)
	data.PermissionTypes = permissionTypesList

	return diags
}

// readAuthorizationWithRetry handles the eventual consistency of fetching an authorization by
// retrying a few times.
func readAuthorizationWithRetry(ctx context.Context, client *camunda.ClientWithResponses, authorizationKey string) (*camunda.GetAuthorizationResponse, error) {
	// Reading an authorization is eventually consistent: if it was just created or updated, it
	// may not be immediately reflected through the API. Handle the eventual consistency by
	// retrying a few times with some delay in between until it is found or we time out.
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
			readResp, err := client.GetAuthorizationWithResponse(ctx, authorizationKey)

			if err != nil {
				return nil, "", err
			}

			return readResp, fmt.Sprintf("%d", readResp.StatusCode()), nil
		},

		// Don't wait too long for the first poll
		Delay:      1 * time.Second,
		MinTimeout: 2 * time.Second,
		// Wait at most this duration before considering the authorization has not been found
		Timeout: 30 * time.Second,
	}

	resp, err := createState.WaitForStateContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("timed out while waiting for authorization '%s' to be available: %w", authorizationKey, err)
	}

	r, ok := resp.(*camunda.GetAuthorizationResponse)
	if !ok {
		// This should not happen
		return nil, fmt.Errorf("unexpected type for authorization read response: %T", resp)
	}

	return r, nil
}
