package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var _ datasource.DataSource = &AuthorizationDataSource{}

func NewAuthorizationDataSource() datasource.DataSource {
	return &AuthorizationDataSource{}
}

type AuthorizationDataSource struct {
	client *camunda.ClientWithResponses
}

func (d *AuthorizationDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_authorization"
}

func (d *AuthorizationDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Camunda cluster authorization by its key.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique key of the authorization to look up (string-encoded int64).",
				Required:            true,
				Validators: []validator.String{
					authorizationKeyValidator{},
				},
			},
			"owner_type": schema.StringAttribute{
				MarkdownDescription: "The type of the owner of permissions (USER, CLIENT, ROLE, GROUP, MAPPING_RULE, or UNSPECIFIED).",
				Computed:            true,
			},
			"owner_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the owner of permissions.",
				Computed:            true,
			},
			"resource_type": schema.StringAttribute{
				MarkdownDescription: "The type of resource that the permissions relate to.",
				Computed:            true,
			},
			"permission_types": schema.SetAttribute{
				MarkdownDescription: "The permission types granted by the authorization.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"resource_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the resource the permission relates to (\"*\" matches all resources). " +
					"Null for a property-based authorization.",
				Computed: true,
			},
			"resource_property_name": schema.StringAttribute{
				MarkdownDescription: "The name of the resource property the permission relates to. " +
					"Null for an ID-based authorization.",
				Computed: true,
			},
		},
	}
}

var authorizationKeyPattern = regexp.MustCompile(`^-?[0-9]+$`)

// authorizationKeyValidator validates an authorization key against the API's LongKey
// contract: an optionally negative integer string of 1 to 25 characters.
type authorizationKeyValidator struct{}

func (v authorizationKeyValidator) Description(ctx context.Context) string {
	return "must be an integer string of 1 to 25 characters"
}

func (v authorizationKeyValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v authorizationKeyValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if len(value) > 25 || !authorizationKeyPattern.MatchString(value) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Authorization Key",
			fmt.Sprintf("id %q must be an integer string of 1 to 25 characters.", value),
		)
	}
}

func (d *AuthorizationDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*camunda.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *camunda.ClientWithResponses, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *AuthorizationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var id types.String

	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := readAuthorizationWithRetry(ctx, d.client, id.ValueString())
	if err != nil {
		// readAuthorizationWithRetry keeps polling on 404 and hands back the last response
		// alongside the timeout error, so a final 404 means the authorization does not exist.
		if apiResp != nil && apiResp.StatusCode() == http.StatusNotFound {
			resp.Diagnostics.AddError("Not Found", fmt.Sprintf("No authorization found with key '%s': %s", id.ValueString(), err))
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read authorization '%s', got error: %s", id.ValueString(), err))
		return
	}

	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	data, diags := authorizationResultToModel(ctx, id, apiResp.JSON200)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read authorization data source")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
