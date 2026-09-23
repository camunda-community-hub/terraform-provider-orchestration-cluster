package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccTenantResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccTenantResourceConfig("tenant1", "Foo Bar", "A test tenant"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("tenant1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Foo Bar"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("A test tenant"),
					),
				},
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_tenant.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				// Update the tenant1 resource with different parameters
				Config: providerConfig + testAccTenantResourceConfig("tenant1", "Plop Plip", "An updated test tenant"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("tenant1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Plop Plip"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("An updated test tenant"),
					),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccTenantResourceConfig(tenantId, name, description string) string {
	return fmt.Sprintf(`
resource "camundacluster_tenant" "test" {
  tenant_id   = "%s"
  name        = "%s"
  description = "%s"
}
`, tenantId, name, description)
}
