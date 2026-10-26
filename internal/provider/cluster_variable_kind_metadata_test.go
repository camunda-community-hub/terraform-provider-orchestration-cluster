package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func testAccClusterVariableKindMetadataConfig(kind, metadata string) string {
	kindLine := ""
	if kind != "" {
		kindLine = fmt.Sprintf("kind = %q", kind)
	}
	metadataLine := ""
	if metadata != "" {
		metadataLine = "metadata = " + metadata
	}
	return fmt.Sprintf(`
resource "orchestration_cluster_cluster_variable" "test" {
  name  = "kind-metadata-variable"
  value = jsonencode("camunda.secrets.MY_SECRET")
  %s
  %s
}

data "orchestration_cluster_cluster_variable" "test" {
  name       = orchestration_cluster_cluster_variable.test.name
  depends_on = [orchestration_cluster_cluster_variable.test]
}
`, kindLine, metadataLine)
}

// TestAccClusterVariable_kindAndMetadata covers the default kind, setting and clearing metadata
// in place, creating a SECRET_REFERENCE variable, replacing the variable when the kind changes,
// and the data source reporting both.
func TestAccClusterVariable_kindAndMetadata(t *testing.T) {
	const res = "orchestration_cluster_cluster_variable.test"
	const ds = "data.orchestration_cluster_cluster_variable.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// The kind defaults to JSON and no metadata is reported as null.
			{
				Config: providerConfig + testAccClusterVariableKindMetadataConfig("", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(res, tfjsonpath.New("kind"), knownvalue.StringExact("JSON")),
					statecheck.ExpectKnownValue(res, tfjsonpath.New("metadata"), knownvalue.Null()),
					statecheck.ExpectKnownValue(ds, tfjsonpath.New("kind"), knownvalue.StringExact("JSON")),
					statecheck.ExpectKnownValue(ds, tfjsonpath.New("metadata"), knownvalue.Null()),
				},
			},
			// Metadata is added in place.
			{
				Config: providerConfig + testAccClusterVariableKindMetadataConfig("", `{ team = "platform", env = "test" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(res, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(res, tfjsonpath.New("metadata"), knownvalue.MapExact(map[string]knownvalue.Check{
						"team": knownvalue.StringExact("platform"),
						"env":  knownvalue.StringExact("test"),
					})),
					statecheck.ExpectKnownValue(ds, tfjsonpath.New("metadata").AtMapKey("team"), knownvalue.StringExact("platform")),
				},
			},
			// Removing the metadata clears it in place.
			{
				Config: providerConfig + testAccClusterVariableKindMetadataConfig("", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(res, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(res, tfjsonpath.New("metadata"), knownvalue.Null()),
					statecheck.ExpectKnownValue(ds, tfjsonpath.New("metadata"), knownvalue.Null()),
				},
			},
			// Changing the kind replaces the variable.
			{
				Config: providerConfig + testAccClusterVariableKindMetadataConfig("SECRET_REFERENCE", `{ owner = "me" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(res, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(res, tfjsonpath.New("kind"), knownvalue.StringExact("SECRET_REFERENCE")),
					statecheck.ExpectKnownValue(ds, tfjsonpath.New("kind"), knownvalue.StringExact("SECRET_REFERENCE")),
					statecheck.ExpectKnownValue(ds, tfjsonpath.New("metadata").AtMapKey("owner"), knownvalue.StringExact("me")),
				},
			},
			// Import reads kind and metadata back without a diff.
			{
				ResourceName:            res,
				ImportState:             true,
				ImportStateId:           "GLOBAL/kind-metadata-variable",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"value"},
			},
		},
	})
}

func TestAccClusterVariable_invalidKind(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      providerConfig + testAccClusterVariableKindMetadataConfig("PLAINTEXT", ""),
				ExpectError: regexp.MustCompile(`must be one of: JSON, SECRET_REFERENCE`),
			},
		},
	})
}
