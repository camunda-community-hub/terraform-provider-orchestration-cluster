package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

const clusterVariableAddress = "camundacluster_cluster_variable.test"

func TestAccClusterVariableResource_global(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccClusterVariableGlobalConfig("accvar-global", `jsonencode({ retries = 3 })`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(clusterVariableAddress, tfjsonpath.New("id"), knownvalue.StringExact("GLOBAL/accvar-global")),
					statecheck.ExpectKnownValue(clusterVariableAddress, tfjsonpath.New("scope"), knownvalue.StringExact("GLOBAL")),
					statecheck.ExpectKnownValue(clusterVariableAddress, tfjsonpath.New("tenant_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(clusterVariableAddress, tfjsonpath.New("value"), knownvalue.StringExact(`{"retries":3}`)),
				},
			},
			// ImportState testing
			{
				ResourceName:      clusterVariableAddress,
				ImportState:       true,
				ImportStateId:     "GLOBAL/accvar-global",
				ImportStateVerify: true,
			},
			// Update in place, including a change of the JSON type of the value
			{
				Config: providerConfig + testAccClusterVariableGlobalConfig("accvar-global", `jsonencode("plain text")`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(clusterVariableAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(clusterVariableAddress, tfjsonpath.New("value"), knownvalue.StringExact(`"plain text"`)),
				},
			},
		},
	})
}

func TestAccClusterVariableResource_tenant(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccClusterVariableTenantConfig("accvar-tenant", "accvar-tenant-a", `jsonencode({ enabled = true })`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(clusterVariableAddress, tfjsonpath.New("id"), knownvalue.StringExact("TENANT/accvar-tenant-a/accvar-tenant")),
					statecheck.ExpectKnownValue(clusterVariableAddress, tfjsonpath.New("scope"), knownvalue.StringExact("TENANT")),
					statecheck.ExpectKnownValue(clusterVariableAddress, tfjsonpath.New("tenant_id"), knownvalue.StringExact("accvar-tenant-a")),
				},
			},
			{
				ResourceName:      clusterVariableAddress,
				ImportState:       true,
				ImportStateId:     "TENANT/accvar-tenant-a/accvar-tenant",
				ImportStateVerify: true,
			},
			// Update in place
			{
				Config: providerConfig + testAccClusterVariableTenantConfig("accvar-tenant", "accvar-tenant-a", `jsonencode({ enabled = false })`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(clusterVariableAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(clusterVariableAddress, tfjsonpath.New("value"), knownvalue.StringExact(`{"enabled":false}`)),
				},
			},
			// Changing the tenant replaces the variable and its scope/id follow
			{
				Config: providerConfig + testAccClusterVariableTenantConfig("accvar-tenant", "accvar-tenant-b", `jsonencode({ enabled = false })`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(clusterVariableAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(clusterVariableAddress, tfjsonpath.New("id"), knownvalue.StringExact("TENANT/accvar-tenant-b/accvar-tenant")),
				},
			},
			// Dropping tenant_id turns it into a global variable: replacement, scope GLOBAL
			{
				Config: providerConfig + testAccClusterVariableGlobalConfig("accvar-tenant", `jsonencode({ enabled = false })`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(clusterVariableAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(clusterVariableAddress, tfjsonpath.New("scope"), knownvalue.StringExact("GLOBAL")),
				},
			},
		},
	})
}

// TestAccClusterVariableResource_lifecycle verifies drift detection after an out-of-band
// delete and replacement when the name changes.
func TestAccClusterVariableResource_lifecycle(t *testing.T) {
	runIdentityLifecycleTest(t, identityLifecycleCase{
		address: clusterVariableAddress,
		config: func(id string) string {
			return testAccClusterVariableGlobalConfig(id, `jsonencode("v")`)
		},
		deleteInEngine: func(ctx context.Context, client *camunda.ClientWithResponses, id string) (int, error) {
			return deleteClusterVariable(ctx, client, nil, id)
		},
	})
}

func testAccClusterVariableGlobalConfig(name, value string) string {
	return fmt.Sprintf(`
resource "camundacluster_cluster_variable" "test" {
  name  = %q
  value = %s
}
`, name, value)
}

func testAccClusterVariableTenantConfig(name, tenantId, value string) string {
	return fmt.Sprintf(`
resource "camundacluster_tenant" "test" {
  tenant_id = %[2]q
  name      = %[2]q
}

resource "camundacluster_cluster_variable" "test" {
  name      = %[1]q
  tenant_id = camundacluster_tenant.test.tenant_id
  value     = %[3]s
}
`, name, tenantId, value)
}
