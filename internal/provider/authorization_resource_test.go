package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccAuthorizationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing (ID-based authorization)
			{
				Config: providerConfig + testAccAuthorizationResourceConfig("resource1", "READ"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_authorization.test",
						tfjsonpath.New("owner_id"),
						knownvalue.StringExact("demo"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_authorization.test",
						tfjsonpath.New("owner_type"),
						knownvalue.StringExact("USER"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_authorization.test",
						tfjsonpath.New("resource_type"),
						knownvalue.StringExact("PROCESS_DEFINITION"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_authorization.test",
						tfjsonpath.New("resource_id"),
						knownvalue.StringExact("resource1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_authorization.test",
						tfjsonpath.New("permission_types"),
						knownvalue.ListExact([]knownvalue.Check{
							knownvalue.StringExact("READ"),
						}),
					),
				},
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_authorization.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing (permission_types can be updated in place)
			{
				Config: providerConfig + testAccAuthorizationResourceConfig("resource1", "READ", "UPDATE"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_authorization.test",
						tfjsonpath.New("permission_types"),
						knownvalue.ListExact([]knownvalue.Check{
							knownvalue.StringExact("READ"),
							knownvalue.StringExact("UPDATE"),
						}),
					),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccAuthorizationResourceConfig(resourceId string, permissionTypes ...string) string {
	quoted := make([]string, len(permissionTypes))
	for i, p := range permissionTypes {
		quoted[i] = fmt.Sprintf("%q", p)
	}

	permissionTypesList := "["
	for i, p := range quoted {
		if i > 0 {
			permissionTypesList += ", "
		}
		permissionTypesList += p
	}
	permissionTypesList += "]"

	return fmt.Sprintf(`
resource "camundacluster_authorization" "test" {
  owner_id         = "demo"
  owner_type       = "USER"
  resource_type    = "PROCESS_DEFINITION"
  resource_id      = "%s"
  permission_types = %s
}
`, resourceId, permissionTypesList)
}
