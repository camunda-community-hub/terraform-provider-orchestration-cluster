package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

func TestAccGroupMemberClientResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccGroupMemberClientResourceConfig(),
				Check:  checkGroupClientAssignmentExistsInEngine(),
			},
		},
	})
}

func testAccGroupMemberClientResourceConfig() string {
	return `
resource "camundacluster_group" "clientmembergroup" {
  name = "clientmembergroup"
}

resource "camundacluster_group_member_client" "test" {
  group_id  = camundacluster_group.clientmembergroup.id
  client_id = "test-client"
}
`
}

func checkGroupClientAssignmentExistsInEngine() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "camundacluster_group_member_client" {
				continue
			}
			gid := rs.Primary.Attributes["group_id"]
			cid := rs.Primary.Attributes["client_id"]
			searchResp, err := client.SearchClientsForGroupWithResponse(context.Background(), gid, camunda.SearchClientsForGroupJSONRequestBody{})
			if err != nil {
				return fmt.Errorf("engine API call failed: %w", err)
			}
			if searchResp.StatusCode() != 200 {
				return fmt.Errorf("search clients for group %s failed (HTTP %d)", gid, searchResp.StatusCode())
			}
			var rawResult struct {
				Items []camunda.GroupClientResult `json:"items"`
			}
			if err := json.Unmarshal(searchResp.Body, &rawResult); err != nil {
				return fmt.Errorf("decode failed: %w", err)
			}
			for _, c := range rawResult.Items {
				if c.ClientId == cid {
					return nil
				}
			}
			return fmt.Errorf("client %s not found in group %s in engine", cid, gid)
		}
		return fmt.Errorf("group_member_client resource not found in Terraform state")
	}
}
