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
				Config: providerConfig + testAccGroupResourceConfig("test-group-1", "Test Group 1", ""),
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
				Config: providerConfig + testAccGroupResourceConfig("test-group-1", "Test Group 2", "A description"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_group.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Test Group 2"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_group.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("A description"),
					),
				},
				Check: checkGroupExistsInEngine("Test Group 2"),
			},
		},
	})
}

func testAccGroupResourceConfig(groupId, name, description string) string {
	if description == "" {
		return fmt.Sprintf(`
resource "camundacluster_group" "test" {
  group_id = %q
  name     = %q
}
`, groupId, name)
	}
	return fmt.Sprintf(`
resource "camundacluster_group" "test" {
  group_id    = %q
  name        = %q
  description = %q
}
`, groupId, name, description)
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

			_, err := waitForConsistency(context.Background(), fmt.Sprintf("group %q in engine", groupId), func() (*camunda.GetGroupResponse, bool, error) {
				resp, err := client.GetGroupWithResponse(context.Background(), groupId)
				if err != nil {
					return nil, false, err
				}
				if resp.StatusCode() != 200 || resp.JSON200 == nil {
					return resp, false, nil
				}
				return resp, resp.JSON200.Name == groupName, nil
			})
			if err != nil {
				return fmt.Errorf("group %s not found or not matching in engine: %w", groupId, err)
			}
			return nil
		}
		return fmt.Errorf("group resource not found in Terraform state")
	}
}
