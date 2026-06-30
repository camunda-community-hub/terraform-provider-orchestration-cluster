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

func TestAccRoleMemberGroupResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccRoleMemberGroupResourceConfig(),
				Check:  checkRoleGroupAssignmentExistsInEngine(),
			},
		},
	})
}

func testAccRoleMemberGroupResourceConfig() string {
	return `
resource "camundacluster_role" "rolemembergrouprole" {
  name = "rolemembergrouprole"
}

resource "camundacluster_group" "rolemembertestgroup" {
  name = "rolemembertestgroup"
}

resource "camundacluster_role_member_group" "test" {
  role_id  = camundacluster_role.rolemembergrouprole.id
  group_id = camundacluster_group.rolemembertestgroup.id
}
`
}

func checkRoleGroupAssignmentExistsInEngine() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "camundacluster_role_member_group" {
				continue
			}
			rid := rs.Primary.Attributes["role_id"]
			gid := rs.Primary.Attributes["group_id"]
			searchResp, err := client.SearchGroupsForRoleWithResponse(context.Background(), rid, camunda.SearchGroupsForRoleJSONRequestBody{})
			if err != nil {
				return fmt.Errorf("engine API call failed: %w", err)
			}
			if searchResp.StatusCode() != 200 {
				return fmt.Errorf("search groups for role %s failed (HTTP %d)", rid, searchResp.StatusCode())
			}
			var rawResult struct {
				Items []camunda.RoleGroupResult `json:"items"`
			}
			if err := json.Unmarshal(searchResp.Body, &rawResult); err != nil {
				return fmt.Errorf("decode failed: %w", err)
			}
			for _, g := range rawResult.Items {
				if g.GroupId == gid {
					return nil
				}
			}
			return fmt.Errorf("group %s not found in role %s in engine", gid, rid)
		}
		return fmt.Errorf("role_member_group resource not found in Terraform state")
	}
}
