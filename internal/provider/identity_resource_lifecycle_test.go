package provider

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

// identityLifecycleCase describes a resource whose out-of-band deletion must be
// detected as drift and whose immutable ID must force replacement.
type identityLifecycleCase struct {
	// address is the Terraform address of the resource under test.
	address string

	// config renders the resource configuration for the given ID.
	config func(id string) string

	// deleteInEngine removes the object with the given ID directly through the API,
	// bypassing Terraform, and returns the HTTP status code.
	deleteInEngine func(ctx context.Context, client *camunda.ClientWithResponses, id string) (int, error)
}

// runIdentityLifecycleTest verifies drift detection and ID replacement.
func runIdentityLifecycleTest(t *testing.T, tc identityLifecycleCase) {
	t.Helper()

	const idA, idB = "lifecycle-a", "lifecycle-b"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + tc.config(idA),
			},
			// Drift: the object disappears out-of-band, the plan must recreate it.
			{
				PreConfig: func() { testDeleteOutOfBand(t, tc, idA) },
				Config:    providerConfig + tc.config(idA),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(tc.address, plancheck.ResourceActionCreate),
					},
				},
			},
			// Immutable ID: changing it must replace the resource, not update it.
			{
				Config: providerConfig + tc.config(idB),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(tc.address, plancheck.ResourceActionReplace),
					},
				},
			},
		},
	})
}

// testDeleteOutOfBand deletes the object through the API, then repeats the delete
// until the API reports 404, because reads are eventually consistent and Terraform's
// refresh must not see a stale 200.
func testDeleteOutOfBand(t *testing.T, tc identityLifecycleCase, id string) {
	t.Helper()
	client, err := camunda.NewClientWithResponses(testClusterURL)
	if err != nil {
		t.Fatalf("creating client: %s", err)
	}
	status, err := tc.deleteInEngine(t.Context(), client, id)
	if err != nil {
		t.Fatalf("deleting %s out-of-band: %s", id, err)
	}
	if status != http.StatusNoContent {
		t.Fatalf("deleting %s out-of-band: unexpected HTTP status %d", id, status)
	}
	_, err = waitForConsistency(t.Context(), fmt.Sprintf("%s deletion", id), func() (int, bool, error) {
		status, err := tc.deleteInEngine(t.Context(), client, id)
		return status, status == http.StatusNotFound, err
	})
	if err != nil {
		t.Fatalf("waiting for out-of-band deletion of %s: %s", id, err)
	}
}
