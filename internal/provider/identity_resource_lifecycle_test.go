package provider

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.10"
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

	// getInEngine reads the object with the given ID through the same GET endpoint the
	// resource's Read uses, and returns the HTTP status code.
	getInEngine func(ctx context.Context, client *camunda.ClientWithResponses, id string) (int, error)
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

// testDeleteOutOfBand deletes the object through the API, then waits until the GET the
// resource's Read uses reports it gone. DELETE is strongly consistent but GET is not, so
// waiting on DELETE alone lets Terraform's refresh see a stale 200 and plan a no-op.
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
	err = waitUntilGoneConsecutively(t.Context(), fmt.Sprintf("%s deletion", id), func() (bool, error) {
		status, err := tc.getInEngine(t.Context(), client, id)
		return status == http.StatusNotFound, err
	})
	if err != nil {
		t.Fatalf("waiting for out-of-band deletion of %s: %s", id, err)
	}
}

// goneObservationsRequired is how many consecutive reads must report an object gone before
// an out-of-band removal counts as visible. A single observation can be a projection
// flapping between its old and new state.
const goneObservationsRequired = 2

// waitUntilGoneConsecutively polls gone, which performs the same read the provider uses
// (a GET by ID or a membership search), until it reports true goneObservationsRequired
// times in a row. Any read that still sees the object resets the count.
func waitUntilGoneConsecutively(ctx context.Context, subject string, gone func() (bool, error)) error {
	consecutive := 0
	_, err := waitForConsistency(ctx, nil, subject, func() (bool, bool, error) {
		isGone, err := gone()
		if err != nil {
			return false, false, err
		}
		if isGone {
			consecutive++
		} else {
			consecutive = 0
		}
		return isGone, consecutive >= goneObservationsRequired, nil
	})
	return err
}
