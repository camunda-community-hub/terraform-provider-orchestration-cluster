package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// TestAccRoleDataSource covers the camundacluster_role data source: the raw
// name-search request and returned fields on a successful lookup, the
// zero-match "Not Found" diagnostic, and the multiple-match "Ambiguous
// Lookup" diagnostic.
//
// role_data_source.go's Read has no retry/consistency handling of its own
// around the search call, and the search index is only eventually
// consistent. So each lookup here runs in a TestStep that comes after the
// role(s) it looks up were created (and confirmed to exist) in an earlier
// step, rather than in the same apply as the create, to avoid racing the
// search index.
func TestAccRoleDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create the role used by the successful-lookup case below.
			{
				Config: providerConfig + testAccRoleResourceConfig("test-role-ds-lookup", "DS Lookup Role", "A description for the data source lookup"),
				Check:  checkRoleExistsInEngine("DS Lookup Role"),
			},
			// Successful lookup by name, now that the role is known to exist.
			{
				Config: providerConfig +
					testAccRoleResourceConfig("test-role-ds-lookup", "DS Lookup Role", "A description for the data source lookup") +
					testAccRoleDataSourceConfig("camundacluster_role.test.name"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.camundacluster_role.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("test-role-ds-lookup"),
					),
					statecheck.ExpectKnownValue(
						"data.camundacluster_role.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("A description for the data source lookup"),
					),
				},
			},
			// Not found: no role exists with this name.
			{
				Config:      providerConfig + testAccRoleDataSourceConfig(`"does-not-exist-role-name"`),
				ExpectError: regexp.MustCompile(`No role found with name`),
			},
			// Create two roles sharing the same name to exercise the ambiguous path.
			{
				Config: providerConfig + testAccRoleDuplicateNameResourcesConfig("Duplicate Role Name"),
				Check:  checkRoleExistsInEngine("Duplicate Role Name"),
			},
			// Ambiguous: the name matches more than one role.
			{
				Config: providerConfig +
					testAccRoleDuplicateNameResourcesConfig("Duplicate Role Name") +
					testAccRoleDataSourceConfig(`"Duplicate Role Name"`),
				ExpectError: regexp.MustCompile(`matched \d+ roles`),
			},
		},
	})
}

func testAccRoleDataSourceConfig(nameExpr string) string {
	return fmt.Sprintf(`
data "camundacluster_role" "test" {
  name = %s
}
`, nameExpr)
}

func testAccRoleDuplicateNameResourcesConfig(name string) string {
	return fmt.Sprintf(`
resource "camundacluster_role" "dup_a" {
  role_id = "test-role-ds-dup-a"
  name    = %q
}

resource "camundacluster_role" "dup_b" {
  role_id = "test-role-ds-dup-b"
  name    = %q
}
`, name, name)
}
