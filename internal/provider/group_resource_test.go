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

func TestAccGroupResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccGroupResourceConfig("Test Group 1"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_group.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Test Group 1"),
					),
				},
				Check: checkGroupExistsInEngine("Test Group 1"),
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_group.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: providerConfig + testAccGroupResourceConfig("Test Group 2"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_group.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Test Group 2"),
					),
				},
				Check: checkGroupExistsInEngine("Test Group 2"),
			},
		},
	})
}

func testAccGroupResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "camundacluster_group" "test" {
  name = %q
}
`, name)
}

func checkGroupExistsInEngine(groupName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "camundacluster_group" {
				continue
			}
			groupId := rs.Primary.ID
			resp, err := client.GetGroupWithResponse(context.Background(), groupId)
			if err != nil {
				return fmt.Errorf("engine API call failed: %w", err)
			}
			if resp.StatusCode() != 200 {
				return fmt.Errorf("group %s not found in engine (HTTP %d)", groupId, resp.StatusCode())
			}
			if resp.JSON200 == nil || resp.JSON200.Name != groupName {
				return fmt.Errorf("group name mismatch: expected %s, got %v", groupName, resp.JSON200)
			}
			return nil
		}
		return fmt.Errorf("group resource not found in Terraform state")
	}
}
