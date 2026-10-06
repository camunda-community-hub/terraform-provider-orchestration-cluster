package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// TestAccAuthorizationDataSource covers the camundacluster_authorization data source: a
// successful lookup by key and the "Not Found" error for an unknown key.
func TestAccAuthorizationDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create the authorization used by the successful-lookup case below.
			{
				Config: providerConfig + testAccAuthorizationResourceConfig([]string{"READ_PROCESS_DEFINITION"}),
				Check:  checkAuthorizationExistsInEngine([]string{"READ_PROCESS_DEFINITION"}),
			},
			// Successful lookup by key.
			{
				Config: providerConfig +
					testAccAuthorizationResourceConfig([]string{"READ_PROCESS_DEFINITION"}) +
					testAccAuthorizationDataSourceConfig("camundacluster_authorization.test.id"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.camundacluster_authorization.test", tfjsonpath.New("owner_type"), knownvalue.StringExact("USER")),
					statecheck.ExpectKnownValue("data.camundacluster_authorization.test", tfjsonpath.New("owner_id"), knownvalue.StringExact("demo")),
					statecheck.ExpectKnownValue("data.camundacluster_authorization.test", tfjsonpath.New("resource_type"), knownvalue.StringExact("PROCESS_DEFINITION")),
					statecheck.ExpectKnownValue("data.camundacluster_authorization.test", tfjsonpath.New("resource_id"), knownvalue.StringExact("test-process")),
					statecheck.ExpectKnownValue("data.camundacluster_authorization.test", tfjsonpath.New("resource_property_name"), knownvalue.Null()),
					statecheck.ExpectKnownValue("data.camundacluster_authorization.test", tfjsonpath.New("permission_types"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("READ_PROCESS_DEFINITION"),
					})),
				},
			},
			// Not found: no authorization exists with this key.
			{
				Config:      providerConfigShortConsistency + testAccAuthorizationDataSourceConfig(`"999999999999999"`),
				ExpectError: regexp.MustCompile(`No authorization found with key`),
			},
		},
	})
}

// TestAccAuthorizationDataSource_PropertyBased verifies that a property-based authorization
// is reported with resource_property_name set and resource_id null.
func TestAccAuthorizationDataSource_PropertyBased(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "camundacluster_authorization" "test" {
  owner_type             = "USER"
  owner_id               = "demo"
  resource_type          = "PROCESS_DEFINITION"
  resource_property_name = "customerId"
  permission_types       = ["READ_PROCESS_DEFINITION"]
}
`,
			},
			{
				Config: providerConfig + `
resource "camundacluster_authorization" "test" {
  owner_type             = "USER"
  owner_id               = "demo"
  resource_type          = "PROCESS_DEFINITION"
  resource_property_name = "customerId"
  permission_types       = ["READ_PROCESS_DEFINITION"]
}
` + testAccAuthorizationDataSourceConfig("camundacluster_authorization.test.id"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.camundacluster_authorization.test", tfjsonpath.New("resource_property_name"), knownvalue.StringExact("customerId")),
					statecheck.ExpectKnownValue("data.camundacluster_authorization.test", tfjsonpath.New("resource_id"), knownvalue.Null()),
				},
			},
		},
	})
}

func testAccAuthorizationDataSourceConfig(idExpr string) string {
	return `
data "camundacluster_authorization" "test" {
  id = ` + idExpr + `
}
`
}
