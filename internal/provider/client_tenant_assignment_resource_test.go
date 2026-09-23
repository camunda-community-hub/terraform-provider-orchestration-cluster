package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// TestAccClientTenantAssignmentResource exercises the assignment of an
// externally provisioned OIDC client to a Camunda cluster tenant.
//
// This test targets the "<default>" tenant, which exists by default on a
// Camunda cluster, since tenant management is implemented by a separate,
// parallel resource (see the ground rules in the PR description) that is not
// available in this branch.
func TestAccClientTenantAssignmentResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccClientTenantAssignmentResourceConfig("<default>", "test-client"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_client_tenant_assignment.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("<default>/test-client"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_client_tenant_assignment.test",
						tfjsonpath.New("tenant_id"),
						knownvalue.StringExact("<default>"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_client_tenant_assignment.test",
						tfjsonpath.New("client_id"),
						knownvalue.StringExact("test-client"),
					),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccClientTenantAssignmentResourceConfig(tenantID, clientID string) string {
	return fmt.Sprintf(`
resource "camundacluster_client_tenant_assignment" "test" {
  tenant_id = %[1]q
  client_id = %[2]q
}
`, tenantID, clientID)
}
