package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// TestAccTenantDataSource covers the camundacluster_tenant data source: lookup by tenant ID,
// lookup by name, the not-found errors for both, and the invalid-configuration error. Lookups
// run in a step after the tenant was created in an earlier step.
func TestAccTenantDataSource(t *testing.T) {
	const dataSource = "data.camundacluster_tenant.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccTenantResourceConfig("tenant-ds-lookup", "DS Lookup Tenant", "A description for the data source lookup"),
			},
			// Successful lookup by tenant ID.
			{
				Config: providerConfig +
					testAccTenantResourceConfig("tenant-ds-lookup", "DS Lookup Tenant", "A description for the data source lookup") +
					testAccTenantDataSourceConfig(`tenant_id = camundacluster_tenant.test.tenant_id`),
				ConfigStateChecks: testAccTenantDataSourceChecks(dataSource),
			},
			// Successful lookup by name.
			{
				Config: providerConfig +
					testAccTenantResourceConfig("tenant-ds-lookup", "DS Lookup Tenant", "A description for the data source lookup") +
					testAccTenantDataSourceConfig(`name = camundacluster_tenant.test.name`),
				ConfigStateChecks: testAccTenantDataSourceChecks(dataSource),
			},
			// Not found by tenant ID.
			{
				Config:      providerConfig + testAccTenantDataSourceConfig(`tenant_id = "does-not-exist-tenant"`),
				ExpectError: regexp.MustCompile(`Unable to read tenant`),
			},
			// Not found by name.
			{
				Config:      providerConfig + testAccTenantDataSourceConfig(`name = "does-not-exist-tenant-name"`),
				ExpectError: regexp.MustCompile(`No tenant found with name`),
			},
			// Neither tenant_id nor name set.
			{
				Config:      providerConfig + testAccTenantDataSourceConfig(``),
				ExpectError: regexp.MustCompile(`Exactly one of .tenant_id. or .name. must be set`),
			},
			// Both tenant_id and name set.
			{
				Config:      providerConfig + testAccTenantDataSourceConfig("tenant_id = \"tenant-ds-lookup\"\n  name = \"DS Lookup Tenant\""),
				ExpectError: regexp.MustCompile(`Exactly one of .tenant_id. or .name. must be set`),
			},
		},
	})
}

func testAccTenantDataSourceChecks(address string) []statecheck.StateCheck {
	return []statecheck.StateCheck{
		statecheck.ExpectKnownValue(address, tfjsonpath.New("id"), knownvalue.StringExact("tenant-ds-lookup")),
		statecheck.ExpectKnownValue(address, tfjsonpath.New("tenant_id"), knownvalue.StringExact("tenant-ds-lookup")),
		statecheck.ExpectKnownValue(address, tfjsonpath.New("name"), knownvalue.StringExact("DS Lookup Tenant")),
		statecheck.ExpectKnownValue(address, tfjsonpath.New("description"), knownvalue.StringExact("A description for the data source lookup")),
	}
}

func testAccTenantDataSourceConfig(lookupAttrs string) string {
	return fmt.Sprintf(`
data "camundacluster_tenant" "test" {
  %s
}
`, lookupAttrs)
}
