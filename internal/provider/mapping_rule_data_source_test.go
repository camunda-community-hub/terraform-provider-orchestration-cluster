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

// TestAccMappingRuleDataSource covers the camundacluster_mapping_rule data source: a
// successful lookup by ID and the read error for an unknown ID.
func TestAccMappingRuleDataSource(t *testing.T) {
	const dataSource = "data.camundacluster_mapping_rule.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Found: look up a mapping rule created in the same configuration.
			{
				Config: providerConfig +
					testAccMappingRuleResourceConfig("test-mapping-rule-ds", "ds-admin", "DS Mapping") +
					testAccMappingRuleDataSourceConfig("camundacluster_mapping_rule.test.mapping_rule_id"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(dataSource, tfjsonpath.New("mapping_rule_id"), knownvalue.StringExact("test-mapping-rule-ds")),
					statecheck.ExpectKnownValue(dataSource, tfjsonpath.New("claim_name"), knownvalue.StringExact("groups")),
					statecheck.ExpectKnownValue(dataSource, tfjsonpath.New("claim_value"), knownvalue.StringExact("ds-admin")),
					statecheck.ExpectKnownValue(dataSource, tfjsonpath.New("name"), knownvalue.StringExact("DS Mapping")),
				},
			},
			// Not found: no mapping rule exists with this ID.
			{
				PreConfig:   shortConsistencyTimeout(t),
				Config:      providerConfig + testAccMappingRuleDataSourceConfig(`"does-not-exist-mapping-rule"`),
				ExpectError: regexp.MustCompile(`Unable to read mapping rule`),
			},
		},
	})
}

func testAccMappingRuleDataSourceConfig(idExpr string) string {
	return fmt.Sprintf(`
data "camundacluster_mapping_rule" "test" {
  mapping_rule_id = %s
}
`, idExpr)
}
