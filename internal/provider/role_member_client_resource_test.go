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

// TestAccRoleMemberClientResource_driftDetection verifies that if the
// client is unassigned from the role out-of-band (outside Terraform),
// Read() detects the drift and removes the resource from state, causing
// Terraform to plan to recreate it.
func TestAccRoleMemberClientResource_driftDetection(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccRoleMemberClientResourceConfig(),
				Check:  checkRoleClientAssignmentExistsInEngine(),
			},
			// Unassign the client out-of-band, then re-plan/apply the same
			// config and expect Terraform to detect the drift and plan to
			// recreate the now-missing resource.
			{
				PreConfig:          unassignRoleMemberClientOutOfBand("rolememberclientrole", "test-client-for-role"),
				Config:             providerConfig + testAccRoleMemberClientResourceConfig(),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func unassignRoleMemberClientOutOfBand(roleId, clientId string) func() {
	return func() {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			panic(err)
		}
		if _, err := client.UnassignRoleFromClientWithResponse(context.Background(), roleId, clientId); err != nil {
			panic(err)
		}

		// The unassignment itself is strongly consistent, but the search index used by
		// Read to verify membership lags behind it, same as everywhere else in this
		// provider. Wait for the removal to actually be visible before letting
		// Terraform's own refresh run, or it can still see the (about-to-be-gone)
		// membership and report an empty plan instead of detecting the drift.
		if _, err := waitForConsistency(context.Background(), fmt.Sprintf("role %q client %q unassignment", roleId, clientId), func() (bool, bool, error) {
			found, err := searchAllRoleClients(context.Background(), client, roleId, clientId)
			if err != nil {
				return false, false, err
			}
			return !found, !found, nil
		}); err != nil {
			panic(err)
		}
	}
}

func testAccRoleMemberClientResourceConfig() string {
	return `
resource "camundacluster_role" "rolememberclientrole" {
  role_id = "rolememberclientrole"
  name    = "rolememberclientrole"
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

			_, err := waitForConsistency(context.Background(), fmt.Sprintf("client %q in role %q in engine", cid, rid), func() (bool, bool, error) {
				searchResp, err := client.SearchClientsForRoleWithResponse(context.Background(), rid, camunda.SearchClientsForRoleJSONRequestBody{})
				if err != nil {
					return false, false, err
				}
				if searchResp.StatusCode() != 200 {
					return false, false, nil
				}
				var rawResult struct {
					Items []camunda.RoleClientResult `json:"items"`
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
				return fmt.Errorf("client %s not found in role %s in engine: %w", cid, rid, err)
			}
			return nil
		}
		return fmt.Errorf("role_member_client resource not found in Terraform state")
	}
}
