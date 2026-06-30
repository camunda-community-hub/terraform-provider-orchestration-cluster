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

func TestAccRoleMemberClientResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccRoleMemberClientResourceConfig(),
				Check:  checkRoleClientAssignmentExistsInEngine(),
			},
		},
	})
}

func testAccRoleMemberClientResourceConfig() string {
	return `
resource "camundacluster_role" "rolememberclientrole" {
  name = "rolememberclientrole"
}

resource "camundacluster_role_member_client" "test" {
  role_id   = camundacluster_role.rolememberclientrole.id
  client_id = "test-client-for-role"
}
`
}

func checkRoleClientAssignmentExistsInEngine() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "camundacluster_role_member_client" {
				continue
			}
			rid := rs.Primary.Attributes["role_id"]
			cid := rs.Primary.Attributes["client_id"]
			searchResp, err := client.SearchClientsForRoleWithResponse(context.Background(), rid, camunda.SearchClientsForRoleJSONRequestBody{})
			if err != nil {
				return fmt.Errorf("engine API call failed: %w", err)
			}
			if searchResp.StatusCode() != 200 {
				return fmt.Errorf("search clients for role %s failed (HTTP %d)", rid, searchResp.StatusCode())
			}
			var rawResult struct {
				Items []camunda.RoleClientResult `json:"items"`
			}
			if err := json.Unmarshal(searchResp.Body, &rawResult); err != nil {
				return fmt.Errorf("decode failed: %w", err)
			}
			for _, c := range rawResult.Items {
				if c.ClientId == cid {
					return nil
				}
			}
			return fmt.Errorf("client %s not found in role %s in engine", cid, rid)
		}
		return fmt.Errorf("role_member_client resource not found in Terraform state")
	}
}
