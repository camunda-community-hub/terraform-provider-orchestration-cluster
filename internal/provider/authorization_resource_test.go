package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

func TestAccAuthorizationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccAuthorizationResourceConfig([]string{"READ_PROCESS_DEFINITION"}),
				Check:  checkAuthorizationExistsInEngine([]string{"READ_PROCESS_DEFINITION"}),
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_authorization.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: providerConfig + testAccAuthorizationResourceConfig([]string{"READ_PROCESS_DEFINITION", "CREATE_PROCESS_INSTANCE"}),
				Check:  checkAuthorizationExistsInEngine([]string{"READ_PROCESS_DEFINITION", "CREATE_PROCESS_INSTANCE"}),
			},
		},
	})
}

func testAccAuthorizationResourceConfig(permissions []string) string {
	quoted := make([]string, len(permissions))
	for i, p := range permissions {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return fmt.Sprintf(`
resource "camundacluster_authorization" "test" {
  owner_type    = "USER"
  owner_id      = "demo"
  resource_type = "PROCESS_DEFINITION"
  permissions   = [%s]
  resource_id   = "test-process"
}
`, strings.Join(quoted, ", "))
}

func checkAuthorizationExistsInEngine(expectedPermissions []string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "camundacluster_authorization" {
				continue
			}
			authId := rs.Primary.ID

			_, err := waitForConsistency(context.Background(), fmt.Sprintf("authorization %q in engine", authId), func() (*camunda.GetAuthorizationResponse, bool, error) {
				resp, err := client.GetAuthorizationWithResponse(context.Background(), authId)
				if err != nil {
					return nil, false, err
				}
				if resp.StatusCode() != 200 || resp.JSON200 == nil {
					return resp, false, nil
				}
				return resp, permissionsMatch(resp.JSON200.PermissionTypes, expectedPermissions), nil
			})
			if err != nil {
				return fmt.Errorf("authorization %s not found or not matching in engine: %w", authId, err)
			}
			return nil
		}
		return fmt.Errorf("authorization resource not found in Terraform state")
	}
}
