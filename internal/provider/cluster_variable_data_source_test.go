package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// TestAccClusterVariableDataSource covers looking up a global and a tenant-scoped variable by
// name, and the "Not Found" diagnostic for a variable that does not exist.
func TestAccClusterVariableDataSource(t *testing.T) {
	const config = `
resource "orchestration_cluster_cluster_variable" "global" {
  name  = "ds-global-variable"
  value = jsonencode({ retries = 3 })
}

resource "orchestration_cluster_tenant" "t" {
  tenant_id = "ds-variable-tenant"
  name      = "ds-variable-tenant"
}

resource "orchestration_cluster_cluster_variable" "tenant" {
  name      = "ds-tenant-variable"
  tenant_id = orchestration_cluster_tenant.t.tenant_id
  value     = jsonencode("hello")
}

data "orchestration_cluster_cluster_variable" "global" {
  name       = orchestration_cluster_cluster_variable.global.name
  depends_on = [orchestration_cluster_cluster_variable.global]
}

data "orchestration_cluster_cluster_variable" "tenant" {
  name      = orchestration_cluster_cluster_variable.tenant.name
  tenant_id = orchestration_cluster_tenant.t.tenant_id
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.orchestration_cluster_cluster_variable.global", tfjsonpath.New("id"), knownvalue.StringExact("GLOBAL/ds-global-variable")),
					statecheck.ExpectKnownValue("data.orchestration_cluster_cluster_variable.global", tfjsonpath.New("scope"), knownvalue.StringExact("GLOBAL")),
					statecheck.ExpectKnownValue("data.orchestration_cluster_cluster_variable.global", tfjsonpath.New("tenant_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue("data.orchestration_cluster_cluster_variable.global", tfjsonpath.New("value"), knownvalue.StringExact(`{"retries":3}`)),
					statecheck.ExpectKnownValue("data.orchestration_cluster_cluster_variable.tenant", tfjsonpath.New("id"), knownvalue.StringExact("TENANT/ds-variable-tenant/ds-tenant-variable")),
					statecheck.ExpectKnownValue("data.orchestration_cluster_cluster_variable.tenant", tfjsonpath.New("scope"), knownvalue.StringExact("TENANT")),
					statecheck.ExpectKnownValue("data.orchestration_cluster_cluster_variable.tenant", tfjsonpath.New("value"), knownvalue.StringExact(`"hello"`)),
				},
			},
			{
				Config: providerConfigShortConsistency + `
data "orchestration_cluster_cluster_variable" "missing" {
  name = "ds-does-not-exist"
}
`,
				ExpectError: regexp.MustCompile(`No GLOBAL cluster variable found with name`),
			},
		},
	})
}
