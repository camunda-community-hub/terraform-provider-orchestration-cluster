package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var _ datasource.DataSource = &RoleDataSource{}

func NewRoleDataSource() datasource.DataSource {
	return &RoleDataSource{}
}

type RoleDataSource struct {
	client *camunda.ClientWithResponses
}

type RoleDataSourceModel struct {
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (d *RoleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (d *RoleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Camunda cluster role by name.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of the role.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The display name of the role to look up. Must match exactly one role.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the role.",
				Computed:            true,
			},
		},
	}
}

func (d *RoleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// roleSearchByNameRequest is sent as a raw JSON body to POST /roles/search instead of via the
// generated SearchRolesJSONRequestBody. The OpenAPI spec defines that request as an allOf of the
// generic SearchQueryRequest plus sibling `filter`/`sort` properties, but the generator collapses
// such allOf-with-siblings schemas into a bare alias of the base type, silently dropping `filter`
// (see RoleFilter, which is generated but never referenced). Building the body by hand is the
// only way to actually filter server-side with the current generated client.
type roleSearchByNameRequest struct {
	Filter struct {
		Name string `json:"name"`
	} `json:"filter"`
}

// roleSearchQueryResult mirrors the actual /roles/search response shape. For the same reason as
// roleSearchByNameRequest above, the generated RoleSearchQueryResult type only carries pagination
// info and drops the `items` array, so SearchRolesResponse.JSON200 can never report any results.
// SearchRolesResponse.Body still holds the raw bytes, which this type decodes.
type roleSearchQueryResult struct {
	Items []camunda.RoleResult `json:"items"`
}

func (d *RoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RoleDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()

	filterReq := roleSearchByNameRequest{}
	filterReq.Filter.Name = name

	bodyBytes, err := json.Marshal(filterReq)
	if err != nil {
		resp.Diagnostics.AddError("Encoding Error", fmt.Sprintf("Unable to encode role search request: %s", err))
		return
	}

	apiResp, err := d.client.SearchRolesWithBodyWithResponse(ctx, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to search for role '%s', got error: %s", name, err))
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Search Error", fmt.Sprintf("Error while searching for role '%s', got HTTP error: %d: %s", name, apiResp.StatusCode(), apiResp.Body))
		return
	}

	var result roleSearchQueryResult
	if err := json.Unmarshal(apiResp.Body, &result); err != nil {
		resp.Diagnostics.AddError("Invalid Response", fmt.Sprintf("Unable to parse role search response: %s", err))
		return
	}

	switch len(result.Items) {
	case 0:
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("No role found with name '%s'.", name))
		return
	case 1:
		// exactly one match
	default:
		resp.Diagnostics.AddError("Ambiguous Lookup", fmt.Sprintf("Name '%s' matched %d roles; role names are not guaranteed unique.", name, len(result.Items)))
		return
	}

	match := result.Items[0]
	data.Id = types.StringValue(match.RoleId)
	data.Name = types.StringValue(match.Name)
	if match.Description != nil {
		data.Description = types.StringValue(*match.Description)
	} else {
		data.Description = types.StringValue("")
	}

	tflog.Trace(ctx, "read role data source")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
