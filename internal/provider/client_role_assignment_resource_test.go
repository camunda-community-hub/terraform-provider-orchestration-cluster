package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// TestAccClientRoleAssignmentResource exercises the assignment of a role to an
// externally provisioned OIDC client.
//
// This test targets the built-in "admin" role, which exists by default on a
// Camunda cluster, since role management is implemented by a separate,
// parallel resource (see the ground rules in the PR description) that is not
// available in this branch.
func TestAccClientRoleAssignmentResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccClientRoleAssignmentResourceConfig("admin", "test-client"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_client_role_assignment.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("admin/test-client"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_client_role_assignment.test",
						tfjsonpath.New("role_id"),
						knownvalue.StringExact("admin"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_client_role_assignment.test",
						tfjsonpath.New("client_id"),
						knownvalue.StringExact("test-client"),
					),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccClientRoleAssignmentResourceConfig(roleID, clientID string) string {
	return fmt.Sprintf(`
resource "camundacluster_client_role_assignment" "test" {
  role_id   = %[1]q
  client_id = %[2]q
}
`, roleID, clientID)
}
