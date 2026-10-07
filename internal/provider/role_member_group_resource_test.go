package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
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
			// ImportState testing
			{
				ResourceName:      "camundacluster_role_member_group.test",
				ImportState:       true,
				ImportStateVerify: true,
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
			// Unassign the group out-of-band, then only refresh state (not
			// apply) against the same config. A Config step here is wrong:
			// its own apply would execute whatever diff its implicit
			// refresh detects, silently recreating the assignment before
			// the framework's mandatory post-apply refresh-plan check runs
			// - which then finds nothing to do and fails with "Expected a
			// non-empty plan, but got an empty refresh plan" deterministically,
			// no matter how long PreConfig waits for consistency. RefreshState
			// only refreshes and plans, never applies, so the detected drift
			// (a plan to recreate the now-missing resource) is what
			// ExpectNonEmptyPlan actually gets to assert on.
			{
				PreConfig:          unassignRoleMemberGroupOutOfBand("rolemembergrouprole", "rolemembertestgroup"),
				RefreshState:       true,
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

		// The unassignment itself is strongly consistent, but the search index used by
		// Read to verify membership lags behind it, same as everywhere else in this
		// provider. Wait for the removal to actually be visible before letting
		// Terraform's own refresh run, or it can still see the (about-to-be-gone)
		// membership and report an empty plan instead of detecting the drift.
		if _, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("role %q group %q unassignment", roleId, groupId), func() (bool, bool, error) {
			found, err := searchAllRoleGroups(context.Background(), client, roleId, groupId)
			if err != nil {
				return false, false, err
			}
			return !found, !found, nil
		}); err != nil {
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

			_, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("group %q in role %q in engine", gid, rid), func() (bool, bool, error) {
				searchResp, err := client.SearchGroupsForRoleWithResponse(context.Background(), rid, camunda.SearchGroupsForRoleJSONRequestBody{})
				if err != nil {
					return false, false, err
				}
				if searchResp.StatusCode() != 200 {
					return false, false, fmt.Errorf("got HTTP error: %d: %s", searchResp.StatusCode(), searchResp.Body)
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
