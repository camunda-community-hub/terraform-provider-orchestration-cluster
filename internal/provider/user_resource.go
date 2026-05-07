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
				MarkdownDescription: "The email of the user.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of a user (the username).",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the user.",
				Required:            true,
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

	// Creating a user is eventually consistent, so we need to wait until the
	// user is actually created before we can read it and save it into the
	// state.
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
			readResp, err := r.client.GetUserWithResponse(ctx, data.Username.ValueString())

			if err != nil {
				return nil, "", err
			}

			return readResp, fmt.Sprintf("%d", readResp.StatusCode()), nil
		},

		Timeout:    1 * time.Minute,
		Delay:      2 * time.Second,
		MinTimeout: 2 * time.Second,
	}

	_, err = createState.WaitForStateContext(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Timeout", fmt.Sprintf("Timed out while waiting for user to be created: %s", err))
		return
	}

	data.Id = types.StringValue(apiResp.JSON201.Username)
	data.Name = types.StringValue(*apiResp.JSON201.Name)
	data.Email = types.StringValue(*apiResp.JSON201.Email)
	data.Username = types.StringValue(apiResp.JSON201.Username)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created user resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UserResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.GetUserWithResponse(ctx, data.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("User with username %s was not found: %d: %s", data.Id.ValueString(), apiResp.StatusCode(), apiResp.Body))
		return
	}

	data.Id = types.StringValue(apiResp.JSON200.Username)
	data.Name = types.StringValue(*apiResp.JSON200.Name)
	data.Email = types.StringValue(*apiResp.JSON200.Email)
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
		Email: data.Email.ValueStringPointer(),
		Name:  data.Name.ValueStringPointer(),
	}
	apiResp, err := r.client.UpdateUserWithResponse(ctx, data.Username.ValueString(), request)

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update user, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Not Updated", fmt.Sprintf("Error while updating user, got HTTP error: %d", apiResp.StatusCode()))
		return
	}

	data.Id = types.StringValue(apiResp.JSON200.Username)
	data.Name = types.StringValue(*apiResp.JSON200.Name)
	data.Email = types.StringValue(*apiResp.JSON200.Email)

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

	if apiResp.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting user, got HTTP error: %d", apiResp.StatusCode()))
		return
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
