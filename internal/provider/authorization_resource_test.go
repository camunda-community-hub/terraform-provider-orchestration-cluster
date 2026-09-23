package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

func TestAccAuthorizationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccAuthorizationResourceConfig(`["READ"]`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_authorization.test",
						tfjsonpath.New("permissions"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("READ"),
						}),
					),
				},
				Check: checkAuthorizationExistsInEngine([]string{"READ"}),
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_authorization.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: providerConfig + testAccAuthorizationResourceConfig(`["READ", "UPDATE"]`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_authorization.test",
						tfjsonpath.New("permissions"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("READ"),
							knownvalue.StringExact("UPDATE"),
						}),
					),
				},
				Check: checkAuthorizationExistsInEngine([]string{"READ", "UPDATE"}),
			},
		},
	})
}

func testAccAuthorizationResourceConfig(permissions string) string {
	return fmt.Sprintf(`
resource "camundacluster_authorization" "test" {
  owner_type    = "USER"
  owner_id      = "demo"
  resource_type = "PROCESS_DEFINITION"
  permissions   = %s
  resource_id   = "test-process"
}
`, permissions)
}

func checkAuthorizationExistsInEngine(expectedPermissions []string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "camundacluster_authorization" {
				continue
			}
			authId := rs.Primary.ID
			resp, err := client.GetAuthorizationWithResponse(context.Background(), authId)
			if err != nil {
				return fmt.Errorf("engine API call failed: %w", err)
			}
			if resp.StatusCode() != 200 {
				return fmt.Errorf("authorization %s not found in engine (HTTP %d)", authId, resp.StatusCode())
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("authorization %s response body is nil", authId)
			}

			actualPermissions := make(map[string]bool, len(resp.JSON200.PermissionTypes))
			for _, p := range resp.JSON200.PermissionTypes {
				actualPermissions[string(p)] = true
			}
			if len(actualPermissions) != len(expectedPermissions) {
				return fmt.Errorf("authorization %s permission count mismatch: expected %v, got %v", authId, expectedPermissions, resp.JSON200.PermissionTypes)
			}
			for _, expected := range expectedPermissions {
				if !actualPermissions[expected] {
					return fmt.Errorf("authorization %s missing expected permission %q, got %v", authId, expected, resp.JSON200.PermissionTypes)
				}
			}
			return nil
		}
		return fmt.Errorf("authorization resource not found in Terraform state")
	}
}
