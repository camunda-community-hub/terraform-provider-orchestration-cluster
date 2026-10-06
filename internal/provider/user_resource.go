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
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

// UserResource defines the resource implementation.
type UserResource struct {
	client *camunda.ClientWithResponses
}

// UserResourceModel describes the resource data model.
type UserResourceModel struct {
	Email    types.String `tfsdk:"email"`
	Id       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Password types.String `tfsdk:"password"`
	Username types.String `tfsdk:"username"`
}

func (r *UserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Camunda cluster user",

		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				MarkdownDescription: "The email of the user. Omit this attribute (or set it to `null`) to indicate no value; an explicitly configured empty string is rejected because the API cannot distinguish it from an absent value.",
				Optional:            true,
				Validators: []validator.String{
					nonEmptyStringValidator{},
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of a user (the username).",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the user. Omit this attribute (or set it to `null`) to indicate no value; an explicitly configured empty string is rejected because the API cannot distinguish it from an absent value.",
				Optional:            true,
				Validators: []validator.String{
					nonEmptyStringValidator{},
				},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "The password of the user.",
				Required:            true,
				Sensitive:           true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "The unique name of a user.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *UserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UserResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.CreateUserJSONRequestBody{
		Email:    data.Email.ValueStringPointer(),
		Name:     data.Name.ValueStringPointer(),
		Password: data.Password.ValueString(),
		Username: data.Username.ValueString(),
	}
	apiResp, err := r.client.CreateUserWithResponse(ctx, request)

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create user, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusCreated {
		resp.Diagnostics.AddError("Not Created", fmt.Sprintf("Error while creating user, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 201 but with no parseable JSON body")
		return
	}

	data.Id = types.StringValue(apiResp.JSON201.Username)
	data.Name = optionalStringValue(apiResp.JSON201.Name)
	data.Email = optionalStringValue(apiResp.JSON201.Email)
	data.Username = types.StringValue(apiResp.JSON201.Username)

	// Persist state from the create response before polling for read consistency, so a
	// polling timeout or transport error doesn't orphan the user the API already created.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := readUserWithRetry(ctx, r.client, apiResp.JSON201.Username); err != nil {
		resp.Diagnostics.AddWarning("Consistency Check Failed", fmt.Sprintf("User %q was created but could not be confirmed readable yet: %s. State was saved from the create response; a later refresh will pick up any drift.", apiResp.JSON201.Username, err))
		return
	}

	tflog.Trace(ctx, "created user resource")
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UserResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.GetUserWithResponse(ctx, data.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user '%s', got error: %s", data.Username.ValueString(), err))
		return
	}

	if apiResp.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Error while reading user '%s', got HTTP error: %d: %s", data.Username.ValueString(), apiResp.StatusCode(), apiResp.Body))
		return
	}

	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	data.Id = types.StringValue(apiResp.JSON200.Username)
	data.Name = optionalStringValue(apiResp.JSON200.Name)
	data.Email = optionalStringValue(apiResp.JSON200.Email)
	data.Username = types.StringValue(apiResp.JSON200.Username)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data UserResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.UpdateUserJSONRequestBody{
		Email:    data.Email.ValueStringPointer(),
		Name:     data.Name.ValueStringPointer(),
		Password: data.Password.ValueStringPointer(),
	}
	apiResp, err := r.client.UpdateUserWithResponse(ctx, data.Username.ValueString(), request)

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update user, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Not Updated", fmt.Sprintf("Error while updating user, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	readResp, err := readUserUntilConsistent(ctx, r.client, data.Username.ValueString(), data.Name.ValueStringPointer(), data.Email.ValueStringPointer())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to confirm user update, got error: %s", err))
		return
	}

	data.Id = types.StringValue(readResp.JSON200.Username)
	data.Name = optionalStringValue(readResp.JSON200.Name)
	data.Email = optionalStringValue(readResp.JSON200.Email)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data UserResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.DeleteUserWithResponse(ctx, data.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete user, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent && apiResp.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting user, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("username"), req, resp)
}

// readUserWithRetry handles the eventual consistency of fetching a user by retrying a few times:
// if a user was just created, it may not be immediately available through the API.
func readUserWithRetry(ctx context.Context, client *camunda.ClientWithResponses, username string) (*camunda.GetUserResponse, error) {
	return readWithRetry(ctx, client, fmt.Sprintf("user %q", username),
		func() (*camunda.GetUserResponse, error) {
			return client.GetUserWithResponse(ctx, username)
		},
		func(readResp *camunda.GetUserResponse) (bool, error) {
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

// readUserUntilConsistent polls GET until it reflects the given name and email, handling the
// same eventual consistency on updates as readUserWithRetry does on creates: the read-side
// projection can briefly return the pre-update values right after a successful PUT, which
// would otherwise make Terraform's post-apply refresh plan non-empty.
func readUserUntilConsistent(ctx context.Context, client *camunda.ClientWithResponses, username string, expectedName, expectedEmail *string) (*camunda.GetUserResponse, error) {
	return readUntilConsistent(ctx, client, fmt.Sprintf("user %q", username),
		func() (*camunda.GetUserResponse, error) {
			return client.GetUserWithResponse(ctx, username)
		},
		func(readResp *camunda.GetUserResponse) (bool, error) {
			switch readResp.StatusCode() {
			case http.StatusOK:
			case http.StatusNotFound:
				return false, nil
			default:
				return false, fmt.Errorf("got HTTP error: %d: %s", readResp.StatusCode(), readResp.Body)
			}

			if readResp.JSON200 == nil {
				return false, nil
			}

			return normalizedDescription(readResp.JSON200.Name) == normalizedDescription(expectedName) &&
				normalizedDescription(readResp.JSON200.Email) == normalizedDescription(expectedEmail), nil
		})
}
