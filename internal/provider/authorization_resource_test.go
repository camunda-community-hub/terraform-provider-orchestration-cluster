package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

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
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_authorization.test",
						tfjsonpath.New("permission_types"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("READ_PROCESS_DEFINITION"),
							knownvalue.StringExact("CREATE_PROCESS_INSTANCE"),
						}),
					),
				},
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
  owner_type       = "USER"
  owner_id         = "demo"
  resource_type    = "PROCESS_DEFINITION"
  permission_types = [%s]
  resource_id      = "test-process"
}
`, strings.Join(quoted, ", "))
}

// TestAccAuthorizationResource_PropertyBased exercises the property-based authorization
// variant (resource_property_name set, resource_id left unset): both Create and, in
// particular, Update, since an earlier bug had Update() unconditionally rebuild an
// ID-based request, silently converting an imported property-based authorization's scope
// to the wildcard "*" resource_id whenever any other field (e.g. permission_types) changed.
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
			// Update testing: changing permission_types must not corrupt the property-based scope
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
  permission_types        = [%s]
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

// TestAccAuthorizationResource_ScopeTransition exercises switching the same authorization
// resource between the ID-based and property-based scope in both directions. This covers a
// bug where authorizationScopePlanModifier's now-removed "already known planned value"
// early-return skipped the sibling-attribute check whenever the framework had already
// prefilled an omitted Optional+Computed attribute's plan from prior state (which happens
// before any plan modifier runs) -- meaning the very case the modifier exists for could
// bypass its logic entirely, leaving the abandoned scope's stale value in the plan.
//
// The final step additionally removes the scope entirely (neither resource_id nor
// resource_property_name configured), covering a related bug where that same modifier
// unconditionally restored the prior state value whenever both scope attributes were omitted,
// which wrongly preserved a previously configured explicit resource_id instead of planning the
// transition to the documented wildcard default.
func TestAccAuthorizationResource_ScopeTransition(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Start ID-based.
			{
				Config: providerConfig + testAccAuthorizationTransitionResourceConfig(false, []string{"READ_PROCESS_DEFINITION"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camundacluster_authorization.test_transition", "resource_id", "test-process"),
					resource.TestCheckNoResourceAttr("camundacluster_authorization.test_transition", "resource_property_name"),
					checkAuthorizationExistsInEngineWithVariant("camundacluster_authorization.test_transition", []string{"READ_PROCESS_DEFINITION"}, authorizationRequestVariant{resourceId: "test-process"}),
				),
			},
			// Transition ID-based -> property-based: resource_id omitted, resource_property_name configured.
			{
				Config: providerConfig + testAccAuthorizationTransitionResourceConfig(true, []string{"READ_PROCESS_DEFINITION"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camundacluster_authorization.test_transition", "resource_property_name", "processDefinitionKey"),
					resource.TestCheckNoResourceAttr("camundacluster_authorization.test_transition", "resource_id"),
					checkAuthorizationExistsInEngineWithVariant("camundacluster_authorization.test_transition", []string{"READ_PROCESS_DEFINITION"}, authorizationRequestVariant{resourcePropertyName: "processDefinitionKey", isPropertyBased: true}),
				),
			},
			// Transition back property-based -> ID-based: resource_property_name omitted, resource_id configured again.
			{
				Config: providerConfig + testAccAuthorizationTransitionResourceConfig(false, []string{"READ_PROCESS_DEFINITION"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camundacluster_authorization.test_transition", "resource_id", "test-process"),
					resource.TestCheckNoResourceAttr("camundacluster_authorization.test_transition", "resource_property_name"),
					checkAuthorizationExistsInEngineWithVariant("camundacluster_authorization.test_transition", []string{"READ_PROCESS_DEFINITION"}, authorizationRequestVariant{resourceId: "test-process"}),
				),
			},
			// Remove the explicit scope entirely: neither resource_id nor resource_property_name
			// configured. This covers a bug where authorizationScopePlanModifier unconditionally
			// restored the prior state value whenever both scope attributes were omitted, on the
			// (wrong) assumption that "neither configured" always means "nothing about scope
			// changed" -- which wrongly preserved the previous explicit resource_id
			// ("test-process") instead of planning the transition to the documented wildcard
			// default ("*").
			{
				Config: providerConfig + testAccAuthorizationTransitionResourceConfigNoScope([]string{"READ_PROCESS_DEFINITION"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("camundacluster_authorization.test_transition", "resource_id", "*"),
					resource.TestCheckNoResourceAttr("camundacluster_authorization.test_transition", "resource_property_name"),
					checkAuthorizationExistsInEngineWithVariant("camundacluster_authorization.test_transition", []string{"READ_PROCESS_DEFINITION"}, authorizationRequestVariant{resourceId: "*"}),
				),
			},
		},
	})
}

func testAccAuthorizationTransitionResourceConfig(useProperty bool, permissions []string) string {
	quoted := make([]string, len(permissions))
	for i, p := range permissions {
		quoted[i] = fmt.Sprintf("%q", p)
	}

	scopeLine := `  resource_id             = "test-process"`
	if useProperty {
		scopeLine = `  resource_property_name  = "processDefinitionKey"`
	}

	return fmt.Sprintf(`
resource "camundacluster_authorization" "test_transition" {
  owner_type       = "USER"
  owner_id         = "demo"
  resource_type    = "PROCESS_DEFINITION"
  permission_types = [%s]
%s
}
`, strings.Join(quoted, ", "), scopeLine)
}

// testAccAuthorizationTransitionResourceConfigNoScope configures the authorization with
// neither resource_id nor resource_property_name set, exercising the documented "defaults to
// wildcard" behavior.
func testAccAuthorizationTransitionResourceConfigNoScope(permissions []string) string {
	quoted := make([]string, len(permissions))
	for i, p := range permissions {
		quoted[i] = fmt.Sprintf("%q", p)
	}

	return fmt.Sprintf(`
resource "camundacluster_authorization" "test_transition" {
  owner_type       = "USER"
  owner_id         = "demo"
  resource_type    = "PROCESS_DEFINITION"
  permission_types = [%s]
}
`, strings.Join(quoted, ", "))
}
