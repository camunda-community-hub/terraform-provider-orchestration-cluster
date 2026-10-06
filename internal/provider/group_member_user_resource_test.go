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

func TestAccGroupMemberUserResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccGroupMemberUserResourceConfig(),
				Check:  checkGroupUserAssignmentExistsInEngine("membertestgroup", "memberuser1"),
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_group_member_user.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccGroupMemberUserResource_driftDetection verifies that if the user
// is unassigned from the group out-of-band (outside Terraform), Read()
// detects the drift and removes the resource from state, causing Terraform
// to plan to recreate it.
func TestAccGroupMemberUserResource_driftDetection(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccGroupMemberUserResourceConfig(),
				Check:  checkGroupUserAssignmentExistsInEngine("membertestgroup", "memberuser1"),
			},
			// Unassign the user out-of-band, then only refresh state (not
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
				PreConfig:          unassignGroupMemberUserOutOfBand("membertestgroup", "memberuser1"),
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func unassignGroupMemberUserOutOfBand(groupId, userId string) func() {
	return func() {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			panic(err)
		}
		if _, err := client.UnassignUserFromGroupWithResponse(context.Background(), groupId, userId); err != nil {
			panic(err)
		}

		// The unassignment itself is strongly consistent, but the search index used by
		// Read to verify membership lags behind it, same as everywhere else in this
		// provider. Wait for the removal to actually be visible before letting
		// Terraform's own refresh run, or it can still see the (about-to-be-gone)
		// membership and report an empty plan instead of detecting the drift.
		if _, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("group %q user %q unassignment", groupId, userId), func() (bool, bool, error) {
			found, err := searchAllGroupUsers(context.Background(), client, groupId, userId)
			if err != nil {
				return false, false, err
			}
			return !found, !found, nil
		}); err != nil {
			panic(err)
		}
	}
}

func testAccGroupMemberUserResourceConfig() string {
	return `
resource "camundacluster_user" "memberuser" {
  username = "memberuser1"
  name     = "Member User"
  email    = "memberuser1@example.com"
  password = "testpass123"
}

resource "camundacluster_group" "membergroup" {
  group_id = "membertestgroup"
  name     = "membertestgroup"
}

resource "camundacluster_group_member_user" "test" {
  group_id = camundacluster_group.membergroup.id
  user_id  = camundacluster_user.memberuser.id
}
`
}

func checkGroupUserAssignmentExistsInEngine(groupId, userId string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "camundacluster_group_member_user" {
				continue
			}
			gid := rs.Primary.Attributes["group_id"]
			uid := rs.Primary.Attributes["user_id"]

			_, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("user %q in group %q in engine", uid, gid), func() (bool, bool, error) {
				searchResp, err := client.SearchUsersForGroupWithResponse(context.Background(), gid, camunda.SearchUsersForGroupJSONRequestBody{})
				if err != nil {
					return false, false, err
				}
				if searchResp.StatusCode() != 200 {
					return false, false, fmt.Errorf("got HTTP error: %d: %s", searchResp.StatusCode(), searchResp.Body)
				}
				var rawResult struct {
					Items []camunda.GroupUserResult `json:"items"`
				}
				if err := json.Unmarshal(searchResp.Body, &rawResult); err != nil {
					return false, false, fmt.Errorf("decode failed: %w", err)
				}
				for _, u := range rawResult.Items {
					if u.Username == uid {
						return true, true, nil
					}
				}
				return false, false, nil
			})
			if err != nil {
				return fmt.Errorf("user %s not found in group %s in engine: %w", uid, gid, err)
			}
			return nil
		}
		return fmt.Errorf("group_member_user resource not found in Terraform state")
	}
}
