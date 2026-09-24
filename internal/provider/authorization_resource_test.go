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

// TestAccAuthorizationResource_PropertyBased exercises the property-based authorization
// variant (resource_property_name set, resource_id left unset): both Create and, in
// particular, Update, since an earlier bug had Update() unconditionally rebuild an
// ID-based request, silently converting an imported property-based authorization's scope
// to the wildcard "*" resource_id whenever any other field (e.g. permissions) changed.
func TestAccAuthorizationResource_PropertyBased(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccAuthorizationPropertyBasedResourceConfig([]string{"READ_PROCESS_DEFINITION"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camundacluster_authorization.test_property", "resource_property_name", "processDefinitionKey"),
					resource.TestCheckNoResourceAttr("camundacluster_authorization.test_property", "resource_id"),
					checkAuthorizationExistsInEngineWithVariant("camundacluster_authorization.test_property", []string{"READ_PROCESS_DEFINITION"}, authorizationRequestVariant{resourcePropertyName: "processDefinitionKey", isPropertyBased: true}),
				),
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_authorization.test_property",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update testing: changing permissions must not corrupt the property-based scope
			// back to an ID-based wildcard grant.
			{
				Config: providerConfig + testAccAuthorizationPropertyBasedResourceConfig([]string{"READ_PROCESS_DEFINITION", "CREATE_PROCESS_INSTANCE"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camundacluster_authorization.test_property", "resource_property_name", "processDefinitionKey"),
					resource.TestCheckNoResourceAttr("camundacluster_authorization.test_property", "resource_id"),
					checkAuthorizationExistsInEngineWithVariant("camundacluster_authorization.test_property", []string{"READ_PROCESS_DEFINITION", "CREATE_PROCESS_INSTANCE"}, authorizationRequestVariant{resourcePropertyName: "processDefinitionKey", isPropertyBased: true}),
				),
			},
		},
	})
}

func testAccAuthorizationPropertyBasedResourceConfig(permissions []string) string {
	quoted := make([]string, len(permissions))
	for i, p := range permissions {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return fmt.Sprintf(`
resource "camundacluster_authorization" "test_property" {
  owner_type              = "USER"
  owner_id                = "demo"
  resource_type           = "PROCESS_DEFINITION"
  permissions             = [%s]
  resource_property_name  = "processDefinitionKey"
}
`, strings.Join(quoted, ", "))
}

func checkAuthorizationExistsInEngine(expectedPermissions []string) resource.TestCheckFunc {
	return checkAuthorizationExistsInEngineWithVariant("camundacluster_authorization.test", expectedPermissions, authorizationRequestVariant{resourceId: "test-process"})
}

// checkAuthorizationExistsInEngineWithVariant verifies that the named authorization
// resource exists in the engine with the expected permissions and, depending on
// expectedVariant, the expected resource_id or resource_property_name scope.
func checkAuthorizationExistsInEngineWithVariant(resourceName string, expectedPermissions []string, expectedVariant authorizationRequestVariant) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}

		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("authorization resource %q not found in Terraform state", resourceName)
		}
		authId := rs.Primary.ID

		if _, err := readAuthorizationUntilConsistent(context.Background(), client, authId, "demo", camunda.OwnerTypeEnum("USER"), camunda.ResourceTypeEnum("PROCESS_DEFINITION"), expectedPermissions, expectedVariant); err != nil {
			return fmt.Errorf("authorization %s not found or not matching in engine: %w", authId, err)
		}
		return nil
	}
}
