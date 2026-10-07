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

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
)

// TestAccRoleDataSource covers the camundacluster_role data source: the raw
// name-search request and returned fields on a successful lookup, the
// zero-match "Not Found" diagnostic, and the multiple-match "Ambiguous
// Lookup" diagnostic.
//
// role_data_source.go's Read polls the eventually consistent search endpoint via
// waitForConsistency, so a lookup racing a just-created role is handled. Each lookup here
// still runs in a TestStep that comes after the role(s) it looks up were created (and
// confirmed to exist) in an earlier step, rather than in the same apply as the create, so
// the zero-match and multiple-match cases below exercise a stable, known set of roles.
func TestAccRoleDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create the role used by the successful-lookup case below.
			{
				Config: providerConfig + testAccRoleResourceConfig("test-role-ds-lookup", "DS Lookup Role", "A description for the data source lookup"),
				Check:  checkRoleExistsInEngine("DS Lookup Role"),
			},
			// Successful lookup by name, now that the role is known to exist.
			{
				Config: providerConfig +
					testAccRoleResourceConfig("test-role-ds-lookup", "DS Lookup Role", "A description for the data source lookup") +
					testAccRoleDataSourceConfig("camundacluster_role.test.name"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.camundacluster_role.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("test-role-ds-lookup"),
					),
					statecheck.ExpectKnownValue(
						"data.camundacluster_role.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("A description for the data source lookup"),
					),
				},
			},
			// Not found: no role exists with this name.
			{
				Config:      providerConfigShortConsistency + testAccRoleDataSourceConfig(`"does-not-exist-role-name"`),
				ExpectError: regexp.MustCompile(`No role found with name`),
			},
			// Create two roles sharing the same name to exercise the ambiguous path.
			// The prerequisite check must wait on /roles/search itself (the same
			// endpoint the data source's Read queries), not on GetRoleWithResponse:
			// that is a separate eventually-consistent projection and can be
			// consistent while search still reflects only one of the two roles.
			{
				Config: providerConfig + testAccRoleDuplicateNameResourcesConfig("Duplicate Role Name"),
				Check:  waitForRoleSearchDuplicates("Duplicate Role Name"),
			},
			// Ambiguous: the name matches more than one role.
			{
				Config: providerConfig +
					testAccRoleDuplicateNameResourcesConfig("Duplicate Role Name") +
					testAccRoleDataSourceConfig(`"Duplicate Role Name"`),
				ExpectError: regexp.MustCompile(`matched \d+ roles`),
			},
		},
	})
}

func testAccRoleDataSourceConfig(nameExpr string) string {
	return fmt.Sprintf(`
data "camundacluster_role" "test" {
  name = %s
}
`, nameExpr)
}

func testAccRoleDuplicateNameResourcesConfig(name string) string {
	return fmt.Sprintf(`
resource "camundacluster_role" "dup_a" {
  role_id = "test-role-ds-dup-a"
  name    = %q
}

resource "camundacluster_role" "dup_b" {
  role_id = "test-role-ds-dup-b"
  name    = %q
}
`, name, name)
}

// waitForRoleSearchDuplicates polls POST /roles/search - the same
// search-by-name mechanism RoleDataSource.Read uses - until it returns at
// least two matches for name. The ambiguous-lookup test step depends on the
// search index (not a by-ID GET, which is a different, separately eventually
// consistent projection) actually reflecting both duplicate roles; otherwise
// the data source's search could still see only one match and the test would
// pass or fail for the wrong reason.
func waitForRoleSearchDuplicates(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}

		_, err = waitForConsistency(context.Background(), nil, fmt.Sprintf("role search for name %q to return duplicates", name), func() ([]camunda.RoleResult, bool, error) {
			filterReq := roleSearchByNameRequest{}
			filterReq.Filter.Name = name

			bodyBytes, err := json.Marshal(filterReq)
			if err != nil {
				return nil, false, err
			}

			apiResp, err := client.SearchRolesWithBodyWithResponse(context.Background(), "application/json", bytes.NewReader(bodyBytes))
			if err != nil {
				return nil, false, err
			}
			if apiResp.StatusCode() != http.StatusOK {
				return nil, false, nil
			}

			var result roleSearchQueryResult
			if err := json.Unmarshal(apiResp.Body, &result); err != nil {
				return nil, false, err
			}

			return result.Items, len(result.Items) >= 2, nil
		})
		if err != nil {
			return fmt.Errorf("role search for name %q did not return both duplicates: %w", name, err)
		}
		return nil
	}
}
