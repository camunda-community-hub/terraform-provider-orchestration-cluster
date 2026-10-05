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

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ClusterTopologyDataSource{}

func NewClusterTopologyDataSource() datasource.DataSource {
	return &ClusterTopologyDataSource{}
}

// ClusterTopologyDataSource defines the data source implementation.
type ClusterTopologyDataSource struct {
	client *camunda.ClientWithResponses
}

// ClusterTopologyDataSourceModel describes the data source data model.
type ClusterTopologyDataSourceModel struct {
	ClusterId         types.String `tfsdk:"cluster_id"`
	ClusterSize       types.Int32  `tfsdk:"cluster_size"`
	GatewayVersion    types.String `tfsdk:"gateway_version"`
	Id                types.String `tfsdk:"id"`
	PartitionsCount   types.Int32  `tfsdk:"partitions_count"`
	ReplicationFactor types.Int32  `tfsdk:"replication_factor"`
}

func (d *ClusterTopologyDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_topology"
}

func (d *ClusterTopologyDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Expose the cluster status of a Camunda cluster",

		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "The cluster Id",
				Optional:            true,
				Computed:            true,
			},
			"cluster_size": schema.Int32Attribute{
				MarkdownDescription: "The number of brokers in the cluster.",
				Optional:            true,
				Computed:            true,
			},
			"gateway_version": schema.StringAttribute{
				MarkdownDescription: "The version of the Zeebe Gateway.",
				Optional:            true,
				Computed:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The cluster Id",
				Computed:            true,
			},
			"partitions_count": schema.Int32Attribute{
				MarkdownDescription: "The number of partitions are spread across the cluster.",
				Optional:            true,
				Computed:            true,
			},
			"replication_factor": schema.Int32Attribute{
				MarkdownDescription: "The configured replication factor for this cluster.",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (d *ClusterTopologyDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*camunda.ClientWithResponses)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *ClusterTopologyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ClusterTopologyDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	topologyResp, err := d.client.GetTopologyWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read cluster status, got error: %s", err))
		return
	}

	if topologyResp.StatusCode() != 200 {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Error while reading cluster status, got HTTP error: %d", topologyResp.StatusCode()))
		return
	}

	// Save the data into the state.
	data.ClusterId = types.StringPointerValue(topologyResp.JSON200.ClusterId)
	data.Id = types.StringValue(data.ClusterId.ValueString())
	data.ClusterSize = types.Int32Value(topologyResp.JSON200.ClusterSize)
	data.GatewayVersion = types.StringValue(topologyResp.JSON200.GatewayVersion)
	data.PartitionsCount = types.Int32Value(topologyResp.JSON200.PartitionsCount)
	data.ReplicationFactor = types.Int32Value(topologyResp.JSON200.ReplicationFactor)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read the user datasource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
