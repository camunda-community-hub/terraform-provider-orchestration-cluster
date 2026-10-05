package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccUserDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + testAccUserDataSourceConfig,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.camundacluster_user.demo",
						tfjsonpath.New("id"),
						knownvalue.StringExact("demo"),
					),
					statecheck.ExpectKnownValue(
						"data.camundacluster_user.demo",
						tfjsonpath.New("username"),
						knownvalue.StringExact("demo"),
					),
					statecheck.ExpectKnownValue(
						"data.camundacluster_user.demo",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Demo User"),
					),
					statecheck.ExpectKnownValue(
						"data.camundacluster_user.demo",
						tfjsonpath.New("email"),
						knownvalue.StringExact("demo@demo.com"),
					),
				},
			},
		},
	})
}

func TestAccUserDataSource_NotFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
data "camundacluster_user" "missing" {
  username = "does-not-exist-user"
}
`,
				ExpectError: regexp.MustCompile(`Unable to read user 'does-not-exist-user'`),
			},
		},
	})
}

const testAccUserDataSourceConfig = `
data "camundacluster_user" "demo" {
  username = "demo"
}
`
