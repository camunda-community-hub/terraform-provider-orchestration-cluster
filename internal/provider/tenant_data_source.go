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

var _ datasource.DataSource = &TenantDataSource{}
var _ datasource.DataSourceWithValidateConfig = &TenantDataSource{}

func NewTenantDataSource() datasource.DataSource {
	return &TenantDataSource{}
}

type TenantDataSource struct {
	client *camunda.ClientWithResponses
}

type TenantDataSourceModel struct {
	Id          types.String `tfsdk:"id"`
	TenantId    types.String `tfsdk:"tenant_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (d *TenantDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant"
}

func (d *TenantDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Camunda cluster tenant by tenant ID or by name. Exactly one of `tenant_id` or `name` must be set.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of the tenant (the tenant ID).",
				Computed:            true,
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of the tenant to look up.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the tenant to look up. Must match exactly one tenant.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the tenant.",
				Computed:            true,
			},
		},
	}
}

// ValidateConfig enforces that exactly one of tenant_id or name is configured. Unknown values
// count as configured, since they may resolve to a value at apply time.
func (d *TenantDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data TenantDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.TenantId.IsNull() == data.Name.IsNull() {
		resp.Diagnostics.AddError(
			"Invalid Lookup Configuration",
			"Exactly one of `tenant_id` or `name` must be set.",
		)
	}
}

func (d *TenantDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// tenantSearchByNameRequest is sent as a raw JSON body to POST /tenants/search instead of via
// the generated SearchTenantsJSONRequestBody, for the same reason as groupSearchByNameRequest:
// the generator drops the `filter` property of the allOf-with-siblings request schema.
type tenantSearchByNameRequest struct {
	Filter struct {
		Name string `json:"name"`
	} `json:"filter"`
}

// tenantSearchQueryResult mirrors the actual /tenants/search response shape; the generated
// TenantSearchQueryResult drops the `items` array (see groupSearchQueryResult).
type tenantSearchQueryResult struct {
	Items []camunda.TenantResult `json:"items"`
}

func (d *TenantDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data TenantDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var tenant *camunda.TenantResult

	if !data.TenantId.IsNull() {
		tenantId := data.TenantId.ValueString()

		readResp, err := readTenantWithRetry(ctx, d.client, tenantId)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read tenant '%s': %s", tenantId, err))
			return
		}
		tenant = readResp.JSON200
	} else {
		var ok bool
		tenant, ok = d.lookupByName(ctx, data.Name.ValueString(), resp)
		if !ok {
			return
		}
	}

	data.Id = types.StringValue(tenant.TenantId)
	data.TenantId = types.StringValue(tenant.TenantId)
	data.Name = types.StringValue(tenant.Name)
	data.Description = optionalStringValue(tenant.Description)

	tflog.Trace(ctx, "read tenant data source")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// lookupByName resolves a tenant by exact name via POST /tenants/search, following the same
// stabilization and uniqueness handling as GroupDataSource.Read (see the comments there). It
// reports diagnostics on resp and returns false on any failure.
func (d *TenantDataSource) lookupByName(ctx context.Context, name string, resp *datasource.ReadResponse) (*camunda.TenantResult, bool) {
	items, matchCount, hardErr, err := searchByNameUntilStable(ctx, fmt.Sprintf("tenant named %q", name),
		func() ([]camunda.TenantResult, error) {
			filterReq := tenantSearchByNameRequest{}
			filterReq.Filter.Name = name

			bodyBytes, err := json.Marshal(filterReq)
			if err != nil {
				return nil, fmt.Errorf("unable to encode tenant search request: %w", err)
			}

			apiResp, err := d.client.SearchTenantsWithBodyWithResponse(ctx, "application/json", bytes.NewReader(bodyBytes))
			if err != nil {
				return nil, err
			}

			if apiResp.StatusCode() != http.StatusOK {
				return nil, fmt.Errorf("got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body)
			}

			var result tenantSearchQueryResult
			if err := json.Unmarshal(apiResp.Body, &result); err != nil {
				return nil, fmt.Errorf("unable to parse tenant search response: %w", err)
			}

			return result.Items, nil
		},
		func(item camunda.TenantResult) string { return item.TenantId })
	if hardErr != nil {
		resp.Diagnostics.AddError("Search Error", fmt.Sprintf("Unable to search for tenant named '%s': %s", name, hardErr))
		return nil, false
	}
	if err != nil {
		if matchCount == 0 {
			resp.Diagnostics.AddError("Not Found", fmt.Sprintf("No tenant found with name '%s': %s", name, err))
		} else {
			resp.Diagnostics.AddError("Search Error", fmt.Sprintf("Search for tenant named '%s' did not stabilize: %s", name, err))
		}
		return nil, false
	}

	if len(items) > 1 {
		resp.Diagnostics.AddError("Ambiguous Lookup", fmt.Sprintf("Name '%s' matched %d tenants; tenant names are not guaranteed unique.", name, len(items)))
		return nil, false
	}

	readResp, err := readUntilConsistent(ctx, fmt.Sprintf("tenant named %q", name),
		func() (*camunda.GetTenantResponse, error) {
			return readTenantWithRetry(ctx, d.client, items[0].TenantId)
		},
		func(readResp *camunda.GetTenantResponse) (bool, error) {
			return readResp.JSON200 != nil && readResp.JSON200.Name == name, nil
		})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read tenant '%s', got error: %s", items[0].TenantId, err))
		return nil, false
	}

	return readResp.JSON200, true
}
