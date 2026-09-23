package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccGroupResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccGroupResourceConfig("group1", "Foo Bar", "A test group"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_group.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("group1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_group.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Foo Bar"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_group.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("A test group"),
					),
				},
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_group.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				// Update the group1 resource with different parameters
				Config: providerConfig + testAccGroupResourceConfig("group1", "Plop Plip", "An updated test group"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_group.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("group1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_group.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Plop Plip"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_group.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("An updated test group"),
					),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccGroupResourceConfig(groupId, name, description string) string {
	return fmt.Sprintf(`
resource "camundacluster_group" "test" {
  group_id    = "%s"
  name        = "%s"
  description = "%s"
}
`, groupId, name, description)
}
