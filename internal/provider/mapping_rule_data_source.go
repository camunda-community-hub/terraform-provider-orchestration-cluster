package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var _ datasource.DataSource = &MappingRuleDataSource{}

func NewMappingRuleDataSource() datasource.DataSource {
	return &MappingRuleDataSource{}
}

// MappingRuleDataSource looks up a mapping rule by its ID via GET /mapping-rules/{mappingRuleId}.
type MappingRuleDataSource struct {
	client *camunda.ClientWithResponses
}

type MappingRuleDataSourceModel struct {
	ClaimName     types.String `tfsdk:"claim_name"`
	ClaimValue    types.String `tfsdk:"claim_value"`
	MappingRuleId types.String `tfsdk:"mapping_rule_id"`
	Name          types.String `tfsdk:"name"`
}

func (d *MappingRuleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mapping_rule"
}

func (d *MappingRuleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Camunda cluster mapping rule by ID.",

		Attributes: map[string]schema.Attribute{
			"claim_name": schema.StringAttribute{
				MarkdownDescription: "The name of the claim to map.",
				Computed:            true,
			},
			"claim_value": schema.StringAttribute{
				MarkdownDescription: "The value of the claim to map.",
				Computed:            true,
			},
			"mapping_rule_id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of the mapping rule to look up.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the mapping rule.",
				Computed:            true,
			},
		},
	}
}

func (d *MappingRuleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *MappingRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data MappingRuleDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.MappingRuleId.ValueString()

	apiResp, err := readMappingRuleWithRetry(ctx, d.client, id)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Mapping Rule", fmt.Sprintf("Unable to read mapping rule '%s', got error: %s", id, err))
		return
	}

	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	data.MappingRuleId = types.StringValue(apiResp.JSON200.MappingRuleId)
	data.ClaimName = types.StringValue(apiResp.JSON200.ClaimName)
	data.ClaimValue = types.StringValue(apiResp.JSON200.ClaimValue)
	data.Name = types.StringValue(apiResp.JSON200.Name)

	tflog.Trace(ctx, "read mapping rule data source")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
