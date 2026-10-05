package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

func TestAccMappingRuleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccMappingRuleResourceConfig("test-mapping-rule", "admin", "Admin Mapping"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_mapping_rule.test",
						tfjsonpath.New("mapping_rule_id"),
						knownvalue.StringExact("test-mapping-rule"),
					),
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
				ResourceName:                         "camundacluster_mapping_rule.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "mapping_rule_id",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["camundacluster_mapping_rule.test"].Primary.Attributes["mapping_rule_id"], nil
				},
			},
			// Update and Read testing
			{
				Config: providerConfig + testAccMappingRuleResourceConfig("test-mapping-rule", "operator", "Operator Mapping"),
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

func testAccMappingRuleResourceConfig(mappingRuleId, claimValue, name string) string {
	return fmt.Sprintf(`
resource "camundacluster_mapping_rule" "test" {
  mapping_rule_id = "%s"
  claim_name      = "groups"
  claim_value     = "%s"
  name            = "%s"
}
`, mappingRuleId, claimValue, name)
}

func TestAccMappingRuleResource_DriftAndReplace(t *testing.T) {
	runIdentityLifecycleTest(t, identityLifecycleCase{
		address: "camundacluster_mapping_rule.test",
		config: func(id string) string {
			return testAccMappingRuleResourceConfig(id, "lifecycle", "Lifecycle Mapping")
		},
		deleteInEngine: func(ctx context.Context, client *camunda.ClientWithResponses, id string) (int, error) {
			resp, err := client.DeleteMappingRuleWithResponse(ctx, id)
			if err != nil {
				return 0, err
			}
			return resp.StatusCode(), nil
		},
	})
}
