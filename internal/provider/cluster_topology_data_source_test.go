package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.10"
)

func readClusterTopology(t *testing.T, status int, body string) (*datasource.ReadResponse, ClusterTopologyDataSourceModel) {
	t.Helper()
	ctx := context.Background()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/topology" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(api.Close)

	client, err := camunda.NewClientWithResponses(api.URL)
	if err != nil {
		t.Fatal(err)
	}

	ds := &ClusterTopologyDataSource{client: client}
	var schemaResp datasource.SchemaResponse
	ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	typ := schemaResp.Schema.Type().TerraformType(ctx)

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(typ, nil)}}
	ds.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: objectOf(typ, nil)},
	}, resp)

	var model ClusterTopologyDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &model)...)
	}
	return resp, model
}

func TestClusterTopologyDataSource_Read(t *testing.T) {
	resp, model := readClusterTopology(t, http.StatusOK, `{
		"brokers": [],
		"clusterId": "cluster-1",
		"clusterSize": 3,
		"gatewayVersion": "8.9.0",
		"lastCompletedChangeId": "1",
		"partitionsCount": 6,
		"replicationFactor": 2
	}`)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if model.Id.ValueString() != "cluster-1" || model.ClusterId.ValueString() != "cluster-1" {
		t.Errorf("id/cluster_id = %q/%q", model.Id.ValueString(), model.ClusterId.ValueString())
	}
	if model.ClusterSize.ValueInt32() != 3 || model.PartitionsCount.ValueInt32() != 6 || model.ReplicationFactor.ValueInt32() != 2 {
		t.Errorf("unexpected numbers: %+v", model)
	}
	if model.GatewayVersion.ValueString() != "8.9.0" {
		t.Errorf("gateway_version = %q", model.GatewayVersion.ValueString())
	}
}

func TestClusterTopologyDataSource_HTTPError(t *testing.T) {
	resp, _ := readClusterTopology(t, http.StatusInternalServerError, `{}`)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

// A cluster without a clusterId (the API schema allows null) must not crash the provider.
func TestClusterTopologyDataSource_NullClusterId(t *testing.T) {
	resp, model := readClusterTopology(t, http.StatusOK, `{
		"brokers": [], "clusterId": null, "clusterSize": 1, "gatewayVersion": "8.9.0",
		"lastCompletedChangeId": "1", "partitionsCount": 1, "replicationFactor": 1
	}`)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !model.ClusterId.IsNull() || !model.Id.IsNull() {
		t.Errorf("cluster_id/id = %v/%v, want null", model.ClusterId, model.Id)
	}
}

func TestClusterTopologyDataSource_Brokers(t *testing.T) {
	resp, model := readClusterTopology(t, http.StatusOK, `{
		"brokers": [{
			"brokerId": "zone-a_0", "nodeId": 0, "host": "broker-0", "port": 26501, "version": "8.10.6",
			"partitions": [
				{"partitionId": 1, "role": "leader", "health": "healthy", "state": "active"},
				{"partitionId": 2, "role": "follower", "health": "unhealthy", "state": "joining"}
			]
		}],
		"clusterId": "cluster-1", "clusterSize": 1, "gatewayVersion": "8.10.6",
		"lastCompletedChangeId": "7", "partitionsCount": 2, "replicationFactor": 1
	}`)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if model.LastCompletedChangeId.ValueString() != "7" {
		t.Errorf("last_completed_change_id = %q", model.LastCompletedChangeId.ValueString())
	}

	brokers := model.Brokers.Elements()
	if len(brokers) != 1 {
		t.Fatalf("got %d brokers, want 1", len(brokers))
	}
	broker := brokers[0].(types.Object).Attributes()
	if broker["broker_id"].(types.String).ValueString() != "zone-a_0" || broker["port"].(types.Int32).ValueInt32() != 26501 ||
		broker["host"].(types.String).ValueString() != "broker-0" || broker["version"].(types.String).ValueString() != "8.10.6" {
		t.Errorf("unexpected broker: %v", broker)
	}
	partitions := broker["partitions"].(types.List).Elements()
	if len(partitions) != 2 {
		t.Fatalf("got %d partitions, want 2", len(partitions))
	}
	second := partitions[1].(types.Object).Attributes()
	if second["partition_id"].(types.Int32).ValueInt32() != 2 || second["role"].(types.String).ValueString() != "follower" ||
		second["health"].(types.String).ValueString() != "unhealthy" || second["state"].(types.String).ValueString() != "joining" {
		t.Errorf("unexpected partition: %v", second)
	}
}
