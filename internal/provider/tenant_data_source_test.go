package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

// TestAccTenantDataSource covers the camundacluster_tenant data source: lookup by tenant ID,
// lookup by name, the not-found errors for both, the invalid-configuration error, and the
// ambiguous-name error. Lookups
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
			// Not found by tenant ID.
			{
				Config:      providerConfigShortConsistency + testAccTenantDataSourceConfig(`tenant_id = "does-not-exist-tenant"`),
				ExpectError: regexp.MustCompile(`Unable to read tenant`),
			},
			// Not found by name.
			{
				Config:      providerConfig + testAccTenantDataSourceConfig(`name = "does-not-exist-tenant-name"`),
				ExpectError: regexp.MustCompile(`No tenant found with name`),
			},
			// Create two tenants sharing the same name to exercise the ambiguous path. The check
			// waits on /tenants/search, the endpoint the data source queries, via
			// waitForTenantSearchDuplicates.
			{
				Config: providerConfig + testAccTenantDuplicateNameResourcesConfig("Duplicate Tenant Name"),
				Check:  waitForTenantSearchDuplicates("Duplicate Tenant Name"),
			},
			// Ambiguous: the name matches more than one tenant.
			{
				Config: providerConfig +
					testAccTenantDuplicateNameResourcesConfig("Duplicate Tenant Name") +
					testAccTenantDataSourceConfig(`name = "Duplicate Tenant Name"`),
				ExpectError: regexp.MustCompile(`matched \d+ tenants`),
			},
		},
	})
}

func testAccTenantDuplicateNameResourcesConfig(name string) string {
	return fmt.Sprintf(`
resource "camundacluster_tenant" "dup_a" {
  tenant_id = "tenant-ds-dup-a"
  name      = %q
}

resource "camundacluster_tenant" "dup_b" {
  tenant_id = "tenant-ds-dup-b"
  name      = %q
}
`, name, name)
}

// waitForTenantSearchDuplicates polls POST /tenants/search until it returns at least two
// matches for name, so the ambiguous-lookup step does not race the search index.
func waitForTenantSearchDuplicates(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}

		_, err = waitForConsistency(context.Background(), nil, fmt.Sprintf("tenant search for name %q to return duplicates", name), func() ([]camunda.TenantResult, bool, error) {
			filterReq := tenantSearchByNameRequest{}
			filterReq.Filter.Name = name

			bodyBytes, err := json.Marshal(filterReq)
			if err != nil {
				return nil, false, err
			}

			apiResp, err := client.SearchTenantsWithBodyWithResponse(context.Background(), "application/json", bytes.NewReader(bodyBytes))
			if err != nil {
				return nil, false, err
			}
			if apiResp.StatusCode() != http.StatusOK {
				return nil, false, nil
			}

			var result tenantSearchQueryResult
			if err := json.Unmarshal(apiResp.Body, &result); err != nil {
				return nil, false, err
			}

			return result.Items, len(result.Items) >= 2, nil
		})
		if err != nil {
			return fmt.Errorf("tenant search for name %q did not return both duplicates: %w", name, err)
		}
		return nil
	}
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
