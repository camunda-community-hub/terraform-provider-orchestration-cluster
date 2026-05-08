package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccExampleDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + testAccExampleDataSourceConfig,
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
					// TODO: test password unset
				},
			},
		},
	})
}

const testAccExampleDataSourceConfig = `
data "camundacluster_user" "demo" {
  username = "demo"
}
`
