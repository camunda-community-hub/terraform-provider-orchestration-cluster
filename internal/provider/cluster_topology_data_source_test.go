package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
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
