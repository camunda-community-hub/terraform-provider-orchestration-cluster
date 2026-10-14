package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
)

var _ datasource.DataSource = &ClusterVariableDataSource{}

func NewClusterVariableDataSource() datasource.DataSource {
	return &ClusterVariableDataSource{}
}

type ClusterVariableDataSource struct {
	client *camunda.ClientWithResponses
}

type ClusterVariableDataSourceModel struct {
	Id       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	TenantId types.String `tfsdk:"tenant_id"`
	Value    types.String `tfsdk:"value"`
	Scope    types.String `tfsdk:"scope"`
}

func (d *ClusterVariableDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_variable"
}

func (d *ClusterVariableDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a variable inside a Camunda cluster by name, either global or scoped to a single tenant. " + docsLinkMarkdown(docClusterVars) + " " +
			"The value is stored in the Terraform state and is **not** marked sensitive: do not store secrets in cluster variables.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The identity of the variable: `GLOBAL/<name>` for a global variable or `TENANT/<tenant_id>/<name>` for a tenant-scoped one.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the cluster variable to look up.",
				Required:            true,
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the tenant the variable is scoped to. Omit it to look up a global variable.",
				Optional:            true,
				Validators:          []validator.String{nonEmptyStringValidator{}},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "The value of the variable as a JSON document, the same way the `orchestration_cluster_cluster_variable` resource reports it. Use `jsondecode()` to read it.",
				Computed:            true,
			},
			"scope": schema.StringAttribute{
				MarkdownDescription: "The scope of the variable: `GLOBAL` or `TENANT`.",
				Computed:            true,
			},
		},
	}
}

func (d *ClusterVariableDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ClusterVariableDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ClusterVariableDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tenantId := tenantIdPointer(data.TenantId)
	name := data.Name.ValueString()

	// A variable created earlier in the same apply may not be readable yet, so a 404 is polled
	// like a pending read and only reported as not found once the timeout elapses.
	notFound := false
	apiResp, err := readWithRetry(ctx, d.client, fmt.Sprintf("cluster variable %q", name),
		func() (clusterVariableResponse, error) {
			notFound = false
			return getClusterVariable(ctx, d.client, tenantId, name)
		},
		func(r clusterVariableResponse) (bool, error) {
			notFound = r.Status == http.StatusNotFound
			if notFound {
				return false, nil
			}
			if r.Status != http.StatusOK {
				return false, fmt.Errorf("got HTTP error: %d: %s", r.Status, r.Body)
			}
			return true, nil
		})
	if err != nil {
		if notFound {
			resp.Diagnostics.AddError("Not Found", fmt.Sprintf("No %s cluster variable found with name '%s': %s", clusterVariableScope(tenantId), name, err))
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read cluster variable '%s', got error: %s", name, err))
		return
	}

	if apiResp.Result == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	// The generated client drops the value field, so it is decoded from the raw body.
	value, err := parseClusterVariableValue(apiResp.Body)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Response", fmt.Sprintf("Unable to read the value of cluster variable '%s': %s", name, err))
		return
	}

	data.Id = types.StringValue(clusterVariableId(apiResp.Result.TenantId, apiResp.Result.Name))
	data.Name = types.StringValue(apiResp.Result.Name)
	data.TenantId = optionalStringValue(apiResp.Result.TenantId)
	data.Scope = types.StringValue(string(apiResp.Result.Scope))
	data.Value = types.StringValue(value)

	tflog.Trace(ctx, "read cluster variable data source")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
