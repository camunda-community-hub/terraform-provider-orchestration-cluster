package provider

import (
	"context"
	"fmt"
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
				Config: providerConfig + testAccAuthorizationResourceConfig(),
				Check:  checkAuthorizationExistsInEngine(),
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_authorization.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccAuthorizationResourceConfig() string {
	return `
resource "camundacluster_authorization" "test" {
  owner_type    = "USER"
  owner_id      = "demo"
  resource_type = "PROCESS_DEFINITION"
  permissions   = ["READ"]
  resource_id   = "test-process"
}
`
}

func checkAuthorizationExistsInEngine() resource.TestCheckFunc {
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
			resp, err := client.GetAuthorizationWithResponse(context.Background(), authId)
			if err != nil {
				return fmt.Errorf("engine API call failed: %w", err)
			}
			if resp.StatusCode() != 200 {
				return fmt.Errorf("authorization %s not found in engine (HTTP %d)", authId, resp.StatusCode())
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("authorization %s response body is nil", authId)
			}
			return nil
		}
		return fmt.Errorf("authorization resource not found in Terraform state")
	}
}
