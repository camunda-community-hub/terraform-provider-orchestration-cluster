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

func TestAccRoleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccRoleResourceConfig("Test Role 1", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_role.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Test Role 1"),
					),
				},
				Check: checkRoleExistsInEngine("Test Role 1"),
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: providerConfig + testAccRoleResourceConfig("Test Role 2", "A description"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_role.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Test Role 2"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_role.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("A description"),
					),
				},
				Check: checkRoleExistsInEngine("Test Role 2"),
			},
		},
	})
}

func testAccRoleResourceConfig(name, description string) string {
	if description == "" {
		return fmt.Sprintf(`
resource "camundacluster_role" "test" {
  name = %q
}
`, name)
	}
	return fmt.Sprintf(`
resource "camundacluster_role" "test" {
  name        = %q
  description = %q
}
`, name, description)
}

func checkRoleExistsInEngine(roleName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "camundacluster_role" {
				continue
			}
			roleId := rs.Primary.ID

			_, err := waitForConsistency(context.Background(), fmt.Sprintf("role %q in engine", roleId), func() (*camunda.GetRoleResponse, bool, error) {
				resp, err := client.GetRoleWithResponse(context.Background(), roleId)
				if err != nil {
					return nil, false, err
				}
				if resp.StatusCode() != 200 || resp.JSON200 == nil {
					return resp, false, nil
				}
				return resp, resp.JSON200.Name == roleName, nil
			})
			if err != nil {
				return fmt.Errorf("role %s not found or not matching in engine: %w", roleId, err)
			}
			return nil
		}
		return fmt.Errorf("role resource not found in Terraform state")
	}
}
