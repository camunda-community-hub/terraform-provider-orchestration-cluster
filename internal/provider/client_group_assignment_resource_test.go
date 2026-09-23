package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// TestAccClientGroupAssignmentResource exercises the assignment of an externally
// provisioned OIDC client to a Camunda cluster group.
//
// This test assumes a group with ID "test-group" already exists on the target
// cluster, since group management is implemented by a separate, parallel
// resource (see the ground rules in the PR description) that is not available
// in this branch.
func TestAccClientGroupAssignmentResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccClientGroupAssignmentResourceConfig("test-group", "test-client"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_client_group_assignment.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("test-group/test-client"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_client_group_assignment.test",
						tfjsonpath.New("group_id"),
						knownvalue.StringExact("test-group"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_client_group_assignment.test",
						tfjsonpath.New("client_id"),
						knownvalue.StringExact("test-client"),
					),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccClientGroupAssignmentResourceConfig(groupID, clientID string) string {
	return fmt.Sprintf(`
resource "camundacluster_client_group_assignment" "test" {
  group_id  = %[1]q
  client_id = %[2]q
}
`, groupID, clientID)
}
