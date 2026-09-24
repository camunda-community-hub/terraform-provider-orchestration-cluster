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

// TestAccGroupDataSource covers the camundacluster_group data source: a
// successful name lookup, the zero-match "Not Found" error, and the
// multiple-match "Ambiguous Lookup" error.
//
// group_data_source.go's Read has no retry/consistency handling of its own
// around the search call, and the search index is only eventually
// consistent. So each lookup here runs in a TestStep that comes after the
// group(s) it looks up were created (and confirmed to exist) in an earlier
// step, rather than in the same apply as the create, to avoid racing the
// search index.
func TestAccGroupDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create the group used by the successful-lookup case below.
			{
				Config: providerConfig + testAccGroupResourceConfig("test-group-ds-lookup", "DS Lookup Group", "A description for the data source lookup"),
				Check:  checkGroupExistsInEngine("DS Lookup Group"),
			},
			// Successful lookup by name, now that the group is known to exist.
			{
				Config: providerConfig +
					testAccGroupResourceConfig("test-group-ds-lookup", "DS Lookup Group", "A description for the data source lookup") +
					testAccGroupDataSourceConfig("camundacluster_group.test.name"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.camundacluster_group.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("test-group-ds-lookup"),
					),
					statecheck.ExpectKnownValue(
						"data.camundacluster_group.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("A description for the data source lookup"),
					),
				},
			},
			// Not found: no group exists with this name.
			{
				Config:      providerConfig + testAccGroupDataSourceConfig(`"does-not-exist-group-name"`),
				ExpectError: regexp.MustCompile(`No group found with name`),
			},
			// Create two groups sharing the same name to exercise the ambiguous path.
			{
				Config: providerConfig + testAccGroupDuplicateNameResourcesConfig("Duplicate Group Name"),
				Check:  checkGroupExistsInEngine("Duplicate Group Name"),
			},
			// Ambiguous: the name matches more than one group.
			{
				Config: providerConfig +
					testAccGroupDuplicateNameResourcesConfig("Duplicate Group Name") +
					testAccGroupDataSourceConfig(`"Duplicate Group Name"`),
				ExpectError: regexp.MustCompile(`matched \d+ groups`),
			},
		},
	})
}

func testAccGroupDataSourceConfig(nameExpr string) string {
	return fmt.Sprintf(`
data "camundacluster_group" "test" {
  name = %s
}
`, nameExpr)
}

func testAccGroupDuplicateNameResourcesConfig(name string) string {
	return fmt.Sprintf(`
resource "camundacluster_group" "dup_a" {
  group_id = "test-group-ds-dup-a"
  name     = %q
}

resource "camundacluster_group" "dup_b" {
  group_id = "test-group-ds-dup-b"
  name     = %q
}
`, name, name)
}
