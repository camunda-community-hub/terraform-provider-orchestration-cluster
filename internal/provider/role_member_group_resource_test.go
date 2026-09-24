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

// TestAccRoleMemberGroupResource_driftDetection verifies that if the group
// is unassigned from the role out-of-band (outside Terraform), Read()
// detects the drift and removes the resource from state, causing Terraform
// to plan to recreate it.
func TestAccRoleMemberGroupResource_driftDetection(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccRoleMemberGroupResourceConfig(),
				Check:  checkRoleGroupAssignmentExistsInEngine(),
			},
			// Unassign the group out-of-band, then re-plan/apply the same
			// config and expect Terraform to detect the drift and plan to
			// recreate the now-missing resource.
			{
				PreConfig:          unassignRoleMemberGroupOutOfBand("rolemembergrouprole", "rolemembertestgroup"),
				Config:             providerConfig + testAccRoleMemberGroupResourceConfig(),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func unassignRoleMemberGroupOutOfBand(roleId, groupId string) func() {
	return func() {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			panic(err)
		}
		if _, err := client.UnassignRoleFromGroupWithResponse(context.Background(), roleId, groupId); err != nil {
			panic(err)
		}
	}
}

func testAccRoleMemberGroupResourceConfig() string {
	return `
resource "camundacluster_role" "rolemembergrouprole" {
  role_id = "rolemembergrouprole"
  name    = "rolemembergrouprole"
}

resource "camundacluster_group" "rolemembertestgroup" {
  group_id = "rolemembertestgroup"
  name     = "rolemembertestgroup"
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

			_, err := waitForConsistency(context.Background(), fmt.Sprintf("group %q in role %q in engine", gid, rid), func() (bool, bool, error) {
				searchResp, err := client.SearchGroupsForRoleWithResponse(context.Background(), rid, camunda.SearchGroupsForRoleJSONRequestBody{})
				if err != nil {
					return false, false, err
				}
				if searchResp.StatusCode() != 200 {
					return false, false, nil
				}
				var rawResult struct {
					Items []camunda.RoleGroupResult `json:"items"`
				}
				if err := json.Unmarshal(searchResp.Body, &rawResult); err != nil {
					return false, false, fmt.Errorf("decode failed: %w", err)
				}
				for _, g := range rawResult.Items {
					if g.GroupId == gid {
						return true, true, nil
					}
				}
				return false, false, nil
			})
			if err != nil {
				return fmt.Errorf("group %s not found in role %s in engine: %w", gid, rid, err)
			}
			return nil
		}
		return fmt.Errorf("role_member_group resource not found in Terraform state")
	}
}
