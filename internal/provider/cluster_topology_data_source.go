package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.10"
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

	LastCompletedChangeId types.String `tfsdk:"last_completed_change_id"`
	Brokers               types.List   `tfsdk:"brokers"`
}

var topologyPartitionAttrTypes = map[string]attr.Type{
	"partition_id": types.Int32Type,
	"role":         types.StringType,
	"health":       types.StringType,
	"state":        types.StringType,
}

var topologyBrokerAttrTypes = map[string]attr.Type{
	"broker_id":  types.StringType,
	"node_id":    types.Int32Type,
	"host":       types.StringType,
	"port":       types.Int32Type,
	"version":    types.StringType,
	"partitions": types.ListType{ElemType: types.ObjectType{AttrTypes: topologyPartitionAttrTypes}},
}

func (d *ClusterTopologyDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_topology"
}

func (d *ClusterTopologyDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Exposes the status and topology of a Camunda cluster. " + docsLinkMarkdown(docClusterAdmin),

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
			"last_completed_change_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the last completed change of the cluster configuration.",
				Computed:            true,
			},
			"brokers": schema.ListNestedAttribute{
				MarkdownDescription: "The brokers that are part of the cluster.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"broker_id": schema.StringAttribute{
							MarkdownDescription: "The unique (within a cluster) broker identifier. For a cluster that is not zone-aware it is the node ID as a string; for a zone-aware cluster it has the form `<zone name>_<node ID>`.",
							Computed:            true,
						},
						"node_id": schema.Int32Attribute{
							MarkdownDescription: "The node ID of the broker. Deprecated by the API in favor of `broker_id`, because it is only unique within its zone in a zone-aware cluster.",
							Computed:            true,
						},
						"host": schema.StringAttribute{
							MarkdownDescription: "The hostname for reaching the broker.",
							Computed:            true,
						},
						"port": schema.Int32Attribute{
							MarkdownDescription: "The port for reaching the broker.",
							Computed:            true,
						},
						"version": schema.StringAttribute{
							MarkdownDescription: "The broker version.",
							Computed:            true,
						},
						"partitions": schema.ListNestedAttribute{
							MarkdownDescription: "The partitions managed or replicated on the broker.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"partition_id": schema.Int32Attribute{
										MarkdownDescription: "The unique ID of the partition.",
										Computed:            true,
									},
									"role": schema.StringAttribute{
										MarkdownDescription: "The Raft role of the broker for the partition: `leader`, `follower` or `inactive`.",
										Computed:            true,
									},
									"health": schema.StringAttribute{
										MarkdownDescription: "The health of the partition: `healthy`, `unhealthy` or `dead`.",
										Computed:            true,
									},
									"state": schema.StringAttribute{
										MarkdownDescription: "The operational state of the partition within the cluster configuration, for example `active`.",
										Computed:            true,
									},
								},
							},
						},
					},
				},
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
	data.Id = data.ClusterId
	data.ClusterSize = types.Int32Value(topologyResp.JSON200.ClusterSize)
	data.GatewayVersion = types.StringValue(topologyResp.JSON200.GatewayVersion)
	data.PartitionsCount = types.Int32Value(topologyResp.JSON200.PartitionsCount)
	data.ReplicationFactor = types.Int32Value(topologyResp.JSON200.ReplicationFactor)
	data.LastCompletedChangeId = types.StringValue(topologyResp.JSON200.LastCompletedChangeId)

	brokers := make([]attr.Value, len(topologyResp.JSON200.Brokers))
	for i, broker := range topologyResp.JSON200.Brokers {
		partitions := make([]attr.Value, len(broker.Partitions))
		for j, partition := range broker.Partitions {
			partitions[j] = types.ObjectValueMust(topologyPartitionAttrTypes, map[string]attr.Value{
				"partition_id": types.Int32Value(partition.PartitionId),
				"role":         types.StringValue(string(partition.Role)),
				"health":       types.StringValue(string(partition.Health)),
				"state":        types.StringValue(string(partition.State)),
			})
		}
		brokers[i] = types.ObjectValueMust(topologyBrokerAttrTypes, map[string]attr.Value{
			"broker_id":  types.StringValue(broker.BrokerId),
			"node_id":    types.Int32Value(broker.NodeId),
			"host":       types.StringValue(broker.Host),
			"port":       types.Int32Value(broker.Port),
			"version":    types.StringValue(broker.Version),
			"partitions": types.ListValueMust(types.ObjectType{AttrTypes: topologyPartitionAttrTypes}, partitions),
		})
	}
	data.Brokers = types.ListValueMust(types.ObjectType{AttrTypes: topologyBrokerAttrTypes}, brokers)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read the user datasource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
