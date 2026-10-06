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

func TestAccTenantResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccTenantResourceConfig("tenant1", "Foo Bar", "A test tenant"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("tenant1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Foo Bar"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("A test tenant"),
					),
				},
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_tenant.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				// Update the tenant1 resource with different parameters
				Config: providerConfig + testAccTenantResourceConfig("tenant1", "Plop Plip", "An updated test tenant"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("tenant1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Plop Plip"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("An updated test tenant"),
					),
				},
			},
			// Update and Read testing without a description, to exercise the optional
			// description being cleared on update and read back as null.
			{
				Config: providerConfig + testAccTenantResourceConfigNoDescription("tenant1", "Plop Plip"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("tenant1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Plop Plip"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("description"),
						knownvalue.Null(),
					),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccTenantResourceConfig(tenantId, name, description string) string {
	return fmt.Sprintf(`
resource "camundacluster_tenant" "test" {
  tenant_id   = "%s"
  name        = "%s"
  description = "%s"
}
`, tenantId, name, description)
}

func testAccTenantResourceConfigNoDescription(tenantId, name string) string {
	return fmt.Sprintf(`
resource "camundacluster_tenant" "test" {
  tenant_id = "%s"
  name      = "%s"
}
`, tenantId, name)
}

// TestAccTenantResource_driftDetection verifies that if the tenant is deleted
// out-of-band, Read() removes it from state so Terraform plans to recreate it.
func TestAccTenantResource_driftDetection(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccTenantResourceConfig("drifttenant", "Drift Tenant", "drift"),
			},
			// RefreshState only refreshes and plans, never applies, so the detected drift
			// is what ExpectNonEmptyPlan asserts on (see the group member drift tests).
			{
				PreConfig:          deleteTenantOutOfBand("drifttenant"),
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccTenantResource_tenantIdChangeForcesReplacement verifies that changing
// tenant_id plans a destroy-and-recreate instead of an in-place update.
func TestAccTenantResource_tenantIdChangeForcesReplacement(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccTenantResourceConfig("replacetenant1", "Replace Tenant", "replace"),
			},
			{
				Config: providerConfig + testAccTenantResourceConfig("replacetenant2", "Replace Tenant", "replace"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("camundacluster_tenant.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_tenant.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("replacetenant2"),
					),
				},
			},
		},
	})
}

func deleteTenantOutOfBand(tenantId string) func() {
	return func() {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			panic(err)
		}
		if _, err := client.DeleteTenantWithResponse(context.Background(), tenantId); err != nil {
			panic(err)
		}

		// Wait for the deletion to be visible so Terraform's refresh detects the drift.
		if _, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("tenant %q deletion", tenantId), func() (bool, bool, error) {
			resp, err := client.GetTenantWithResponse(context.Background(), tenantId)
			if err != nil {
				return false, false, err
			}
			gone := resp.StatusCode() == 404
			return gone, gone, nil
		}); err != nil {
			panic(err)
		}
	}
}
