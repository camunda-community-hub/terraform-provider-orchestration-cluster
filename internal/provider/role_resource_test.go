package provider

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
)

func TestAccRoleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccRoleResourceConfig("test-role-1", "Test Role 1", ""),
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
				Config: providerConfig + testAccRoleResourceConfig("test-role-1", "Test Role 2", "A description"),
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
			// Clear description by omitting it from config: description must go back to null,
			// not linger at its previously configured value.
			{
				Config: providerConfig + testAccRoleResourceConfig("test-role-1", "Test Role 2", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_role.test",
						tfjsonpath.New("description"),
						knownvalue.Null(),
					),
				},
				Check: checkRoleExistsInEngine("Test Role 2"),
			},
		},
	})
}

func testAccRoleResourceConfig(roleId, name, description string) string {
	if description == "" {
		return fmt.Sprintf(`
resource "camundacluster_role" "test" {
  role_id = %q
  name    = %q
}
`, roleId, name)
	}
	return fmt.Sprintf(`
resource "camundacluster_role" "test" {
  role_id     = %q
  name        = %q
  description = %q
}
`, roleId, name, description)
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

			_, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("role %q in engine", roleId), func() (*camunda.GetRoleResponse, bool, error) {
				resp, err := client.GetRoleWithResponse(context.Background(), roleId)
				if err != nil {
					return nil, false, err
				}
				switch resp.StatusCode() {
				case http.StatusNotFound:
					return resp, false, nil
				case http.StatusOK:
					if resp.JSON200 == nil {
						return nil, false, fmt.Errorf("got 200 response with unparseable body: %s", resp.Body)
					}
					return resp, resp.JSON200.Name == roleName, nil
				default:
					return nil, false, fmt.Errorf("got HTTP error: %d: %s", resp.StatusCode(), resp.Body)
				}
			})
			if err != nil {
				return fmt.Errorf("role %s not found or not matching in engine: %w", roleId, err)
			}
			return nil
		}
		return fmt.Errorf("role resource not found in Terraform state")
	}
}

func TestAccRoleResource_DriftAndReplace(t *testing.T) {
	runIdentityLifecycleTest(t, identityLifecycleCase{
		address: "camundacluster_role.test",
		config:  func(id string) string { return testAccRoleResourceConfig(id, "Lifecycle Role", "") },
		deleteInEngine: func(ctx context.Context, client *camunda.ClientWithResponses, id string) (int, error) {
			resp, err := client.DeleteRoleWithResponse(ctx, id)
			if err != nil {
				return 0, err
			}
			return resp.StatusCode(), nil
		},
	})
}
