package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccUserResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccUserResourceConfig("user1", "Foo Bar"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_user.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("user1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_user.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Foo Bar"),
					),
				},
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"password", // The password is not importable
				},
			},
			// Update and Read testing
			{
				// Update the test resource with different parameters
				Config: providerConfig + testAccUserResourceConfig("user2", "Plop Plip"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_user.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("user2"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_user.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Plop Plip"),
					),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccUserResourceConfig(username, name string) string {
	return fmt.Sprintf(`
resource "camundacluster_user" "test" {
  username = "%s"
  name     = "%s"
  email    = "%s@example.com"
  password = "test123-%s"
}
`, username, name, username, username)
}
