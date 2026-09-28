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

// TestAccGroupDataSource covers the camundacluster_group data source: a
// successful name lookup, the zero-match "Not Found" error, and the
// multiple-match "Ambiguous Lookup" error.
//
// group_data_source.go's Read polls the eventually consistent search endpoint via
// waitForConsistency, so a lookup racing a just-created group is handled. Each lookup here
// still runs in a TestStep that comes after the group(s) it looks up were created (and
// confirmed to exist) in an earlier step, rather than in the same apply as the create, so
// the zero-match and multiple-match cases below exercise a stable, known set of groups.
func TestAccGroupDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create the group used by the successful-lookup case below.
			{
				Config: providerConfig + testAccGroupResourceConfig("test-group-ds-lookup", "DS Lookup Group", "A description for the data source lookup"),
				Check:  checkGroupExistsInEngine("DS Lookup Group"),
			},
			// Successful lookup by name, now that the group is known to exist.
			{
				Config: providerConfig +
					testAccGroupResourceConfig("test-group-ds-lookup", "DS Lookup Group", "A description for the data source lookup") +
					testAccGroupDataSourceConfig("camundacluster_group.test.name"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.camundacluster_group.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("test-group-ds-lookup"),
					),
					statecheck.ExpectKnownValue(
						"data.camundacluster_group.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("A description for the data source lookup"),
					),
				},
			},
			// Not found: no group exists with this name.
			{
				Config:      providerConfig + testAccGroupDataSourceConfig(`"does-not-exist-group-name"`),
				ExpectError: regexp.MustCompile(`No group found with name`),
			},
			// Create two groups sharing the same name to exercise the ambiguous path.
			// The prerequisite check must wait on /groups/search itself (the same
			// endpoint the data source's Read queries), not on GetGroupWithResponse:
			// that is a separate eventually-consistent projection and can be
			// consistent while search still reflects only one of the two groups.
			{
				Config: providerConfig + testAccGroupDuplicateNameResourcesConfig("Duplicate Group Name"),
				Check:  waitForGroupSearchDuplicates("Duplicate Group Name"),
			},
			// Ambiguous: the name matches more than one group.
			{
				Config: providerConfig +
					testAccGroupDuplicateNameResourcesConfig("Duplicate Group Name") +
					testAccGroupDataSourceConfig(`"Duplicate Group Name"`),
				ExpectError: regexp.MustCompile(`matched \d+ groups`),
			},
		},
	})
}

func testAccGroupDataSourceConfig(nameExpr string) string {
	return fmt.Sprintf(`
data "camundacluster_group" "test" {
  name = %s
}
`, nameExpr)
}

func testAccGroupDuplicateNameResourcesConfig(name string) string {
	return fmt.Sprintf(`
resource "camundacluster_group" "dup_a" {
  group_id = "test-group-ds-dup-a"
  name     = %q
}

resource "camundacluster_group" "dup_b" {
  group_id = "test-group-ds-dup-b"
  name     = %q
}
`, name, name)
}

// waitForGroupSearchDuplicates polls POST /groups/search - the same
// search-by-name mechanism GroupDataSource.Read uses - until it returns at
// least two matches for name. The ambiguous-lookup test step depends on the
// search index (not a by-ID GET, which is a different, separately eventually
// consistent projection) actually reflecting both duplicate groups; otherwise
// the data source's search could still see only one match and the test would
// pass or fail for the wrong reason.
func waitForGroupSearchDuplicates(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}

		_, err = waitForConsistency(context.Background(), fmt.Sprintf("group search for name %q to return duplicates", name), func() ([]camunda.GroupResult, bool, error) {
			filterReq := groupSearchByNameRequest{}
			filterReq.Filter.Name = name

			bodyBytes, err := json.Marshal(filterReq)
			if err != nil {
				return nil, false, err
			}

			apiResp, err := client.SearchGroupsWithBodyWithResponse(context.Background(), "application/json", bytes.NewReader(bodyBytes))
			if err != nil {
				return nil, false, err
			}
			if apiResp.StatusCode() != http.StatusOK {
				return nil, false, nil
			}

			var result groupSearchQueryResult
			if err := json.Unmarshal(apiResp.Body, &result); err != nil {
				return nil, false, err
			}

			return result.Items, len(result.Items) >= 2, nil
		})
		if err != nil {
			return fmt.Errorf("group search for name %q did not return both duplicates: %w", name, err)
		}
		return nil
	}
}
