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

func TestAccTenantMemberClientResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccTenantMemberClientResourceConfig(),
				Check:  checkTenantClientAssignmentExistsInEngine(),
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_tenant_member_client.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccTenantMemberClientResource_driftDetection verifies that if the
// client is unassigned from the tenant out-of-band (outside Terraform),
// Read() detects the drift and removes the resource from state, causing
// Terraform to plan to recreate it.
func TestAccTenantMemberClientResource_driftDetection(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccTenantMemberClientResourceConfig(),
				Check:  checkTenantClientAssignmentExistsInEngine(),
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
				PreConfig:          unassignTenantMemberClientOutOfBand("clientmembertenant", "test-client"),
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func unassignTenantMemberClientOutOfBand(tenantId, clientId string) func() {
	return func() {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			panic(err)
		}
		if _, err := client.UnassignClientFromTenantWithResponse(context.Background(), tenantId, clientId); err != nil {
			panic(err)
		}

		// The unassignment itself is strongly consistent, but the search index used by
		// Read to verify membership lags behind it, same as everywhere else in this
		// provider. Wait for the removal to actually be visible before letting
		// Terraform's own refresh run, or it can still see the (about-to-be-gone)
		// membership and report an empty plan instead of detecting the drift.
		if _, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("tenant %q client %q unassignment", tenantId, clientId), func() (bool, bool, error) {
			found, err := searchAllTenantClients(context.Background(), client, tenantId, clientId)
			if err != nil {
				return false, false, err
			}
			return !found, !found, nil
		}); err != nil {
			panic(err)
		}
	}
}

func testAccTenantMemberClientResourceConfig() string {
	return `
resource "camundacluster_tenant" "clientmembertenant" {
  tenant_id = "clientmembertenant"
  name      = "clientmembertenant"
}

resource "camundacluster_tenant_member_client" "test" {
  tenant_id = camundacluster_tenant.clientmembertenant.id
  client_id = "test-client"
}
`
}

func checkTenantClientAssignmentExistsInEngine() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "camundacluster_tenant_member_client" {
				continue
			}
			tid := rs.Primary.Attributes["tenant_id"]
			cid := rs.Primary.Attributes["client_id"]

			_, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("client %q in tenant %q in engine", cid, tid), func() (bool, bool, error) {
				searchResp, err := client.SearchClientsForTenantWithResponse(context.Background(), tid, camunda.SearchClientsForTenantJSONRequestBody{})
				if err != nil {
					return false, false, err
				}
				if searchResp.StatusCode() != 200 {
					return false, false, fmt.Errorf("got HTTP error: %d: %s", searchResp.StatusCode(), searchResp.Body)
				}
				var rawResult struct {
					Items []camunda.TenantClientResult `json:"items"`
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
				return fmt.Errorf("client %s not found in tenant %s in engine: %w", cid, tid, err)
			}
			return nil
		}
		return fmt.Errorf("tenant_member_client resource not found in Terraform state")
	}
}
