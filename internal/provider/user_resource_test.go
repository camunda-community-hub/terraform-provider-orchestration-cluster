package provider

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

func TestAccUserResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccUserResourceConfig("user1", "Foo Bar"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_user.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("user1"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_user.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Foo Bar"),
					),
				},
			},
			// ImportState testing
			{
				ResourceName:      "camundacluster_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"password", // The password is not importable
				},
			},
			// Update and Read testing
			{
				// Update the user1 resource with different parameters
				Config: providerConfig + testAccUserResourceConfig("user2", "Plop Plip"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"camundacluster_user.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("user2"),
					),
					statecheck.ExpectKnownValue(
						"camundacluster_user.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Plop Plip"),
					),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccUserResourceConfig(username, name string) string {
	return fmt.Sprintf(`
resource "camundacluster_user" "test" {
  username = "%s"
  name     = "%s"
  email    = "%s@example.com"
  password = "test123-%s"
}
`, username, name, username, username)
}

func testAccUserResourceMinimalConfig(username string) string {
	return fmt.Sprintf(`
resource "camundacluster_user" "test" {
  username = %q
  password = "test123-%s"
}
`, username, username)
}

func testAccUserResourceFullConfig(username, name, email, password string) string {
	return fmt.Sprintf(`
resource "camundacluster_user" "test" {
  username = %q
  name     = %q
  email    = %q
  password = %q
}
`, username, name, email, password)
}

func TestAccUserResource_optionalNameAndEmailInPlaceUpdate(t *testing.T) {
	const addr = "camundacluster_user.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccUserResourceMinimalConfig("user-optional"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("name"), knownvalue.Null()),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("email"), knownvalue.Null()),
				},
			},
			{
				Config: providerConfig + testAccUserResourceFullConfig("user-optional", "Foo Bar", "foo@example.com", "test123-user-optional"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("name"), knownvalue.StringExact("Foo Bar")),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("email"), knownvalue.StringExact("foo@example.com")),
				},
			},
			{
				Config: providerConfig + testAccUserResourceFullConfig("user-optional", "Plop Plip", "plop@example.com", "changed-password-1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("name"), knownvalue.StringExact("Plop Plip")),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("email"), knownvalue.StringExact("plop@example.com")),
				},
			},
		},
	})
}

func TestAccUserResource_usernameChangeForcesReplace(t *testing.T) {
	const addr = "camundacluster_user.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccUserResourceConfig("user-replace-a", "Foo Bar"),
			},
			{
				Config: providerConfig + testAccUserResourceConfig("user-replace-b", "Foo Bar"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("id"), knownvalue.StringExact("user-replace-b")),
				},
			},
		},
	})
}

func TestAccUserResource_driftOutOfBandDelete(t *testing.T) {
	const addr = "camundacluster_user.test"
	config := providerConfig + testAccUserResourceConfig("user-drift", "Foo Bar")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				PreConfig: deleteUserOutOfBand(t, "user-drift"),
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("id"), knownvalue.StringExact("user-drift")),
				},
				Check: checkUserExistsInEngine("user-drift"),
			},
		},
	})
}

func deleteUserOutOfBand(t *testing.T, username string) func() {
	return func() {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			t.Fatalf("creating client: %s", err)
		}

		resp, err := client.DeleteUserWithResponse(context.Background(), username)
		if err != nil {
			t.Fatalf("deleting user %q out of band: %s", username, err)
		}
		if resp.StatusCode() != http.StatusNoContent {
			t.Fatalf("deleting user %q out of band: got HTTP %d: %s", username, resp.StatusCode(), resp.Body)
		}

		_, err = waitForConsistency(context.Background(), fmt.Sprintf("user %q deletion", username), func() (*camunda.GetUserResponse, bool, error) {
			getResp, err := client.GetUserWithResponse(context.Background(), username)
			if err != nil {
				return nil, false, err
			}
			return getResp, getResp.StatusCode() == http.StatusNotFound, nil
		})
		if err != nil {
			t.Fatalf("waiting for out-of-band deletion: %s", err)
		}
	}
}

func checkUserExistsInEngine(username string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}

		_, err = readUserWithRetry(context.Background(), client, username)
		return err
	}
}
