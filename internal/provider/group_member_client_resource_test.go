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
			// ImportState testing
			{
				ResourceName:      "camundacluster_group_member_client.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccGroupMemberClientResource_driftDetection verifies that if the
// client is unassigned from the group out-of-band (outside Terraform),
// Read() detects the drift and removes the resource from state, causing
// Terraform to plan to recreate it.
func TestAccGroupMemberClientResource_driftDetection(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccGroupMemberClientResourceConfig(),
				Check:  checkGroupClientAssignmentExistsInEngine(),
			},
			// Unassign the client out-of-band, then only refresh state (not
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
				PreConfig:          unassignGroupMemberClientOutOfBand("clientmembergroup", "test-client"),
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func unassignGroupMemberClientOutOfBand(groupId, clientId string) func() {
	return func() {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			panic(err)
		}
		if _, err := client.UnassignClientFromGroupWithResponse(context.Background(), groupId, clientId); err != nil {
			panic(err)
		}

		// The unassignment itself is strongly consistent, but the search index used by
		// Read to verify membership lags behind it, same as everywhere else in this
		// provider. Wait for the removal to actually be visible before letting
		// Terraform's own refresh run, or it can still see the (about-to-be-gone)
		// membership and report an empty plan instead of detecting the drift.
		if _, err := waitForConsistency(context.Background(), fmt.Sprintf("group %q client %q unassignment", groupId, clientId), func() (bool, bool, error) {
			found, err := searchAllGroupClients(context.Background(), client, groupId, clientId)
			if err != nil {
				return false, false, err
			}
			return !found, !found, nil
		}); err != nil {
			panic(err)
		}
	}
}

func testAccGroupMemberClientResourceConfig() string {
	return `
resource "camundacluster_group" "clientmembergroup" {
  group_id = "clientmembergroup"
  name     = "clientmembergroup"
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

			_, err := waitForConsistency(context.Background(), fmt.Sprintf("client %q in group %q in engine", cid, gid), func() (bool, bool, error) {
				searchResp, err := client.SearchClientsForGroupWithResponse(context.Background(), gid, camunda.SearchClientsForGroupJSONRequestBody{})
				if err != nil {
					return false, false, err
				}
				if searchResp.StatusCode() != 200 {
					return false, false, fmt.Errorf("got HTTP error: %d: %s", searchResp.StatusCode(), searchResp.Body)
				}
				var rawResult struct {
					Items []camunda.GroupClientResult `json:"items"`
				}
				if err := json.Unmarshal(searchResp.Body, &rawResult); err != nil {
					return false, false, fmt.Errorf("decode failed: %w", err)
				}
				for _, c := range rawResult.Items {
					if c.ClientId == cid {
						return true, true, nil
					}
				}
				return false, false, nil
			})
			if err != nil {
				return fmt.Errorf("client %s not found in group %s in engine: %w", cid, gid, err)
			}
			return nil
		}
		return fmt.Errorf("group_member_client resource not found in Terraform state")
	}
}
