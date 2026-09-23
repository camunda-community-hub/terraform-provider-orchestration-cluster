package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccMappingRuleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccMappingRuleResourceConfig("groups", "admin", "Admin Mapping"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_mapping_rule.test",
						tfjsonpath.New("claim_name"),
						knownvalue.StringExact("groups"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_mapping_rule.test",
						tfjsonpath.New("claim_value"),
						knownvalue.StringExact("admin"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_mapping_rule.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Admin Mapping"),
					),
				},
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_mapping_rule.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["camundacluster_mapping_rule.test"].Primary.Attributes["mapping_rule_id"], nil
				},
			},
			// Update and Read testing
			{
				Config: providerConfig + testAccMappingRuleResourceConfig("groups", "operator", "Operator Mapping"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_mapping_rule.test",
						tfjsonpath.New("claim_name"),
						knownvalue.StringExact("groups"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_mapping_rule.test",
						tfjsonpath.New("claim_value"),
						knownvalue.StringExact("operator"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_mapping_rule.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Operator Mapping"),
					),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccMappingRuleResourceConfig(claimName, claimValue, name string) string {
	return fmt.Sprintf(`
resource "camundacluster_mapping_rule" "test" {
  claim_name  = "%s"
  claim_value = "%s"
  name        = "%s"
}
`, claimName, claimValue, name)
}
