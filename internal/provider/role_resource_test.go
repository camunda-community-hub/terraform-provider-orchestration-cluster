package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccRoleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccRoleResourceConfig("role1", "Foo Bar", "A test role"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_role.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("role1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_role.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Foo Bar"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_role.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("A test role"),
					),
				},
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				// Update the role1 resource with different parameters
				Config: providerConfig + testAccRoleResourceConfig("role1", "Plop Plip", "An updated test role"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_role.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("role1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_role.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Plop Plip"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_role.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("An updated test role"),
					),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccRoleResourceConfig(roleId, name, description string) string {
	return fmt.Sprintf(`
resource "camundacluster_role" "test" {
  role_id     = "%s"
  name        = "%s"
  description = "%s"
}
`, roleId, name, description)
}
