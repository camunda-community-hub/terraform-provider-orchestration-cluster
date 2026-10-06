package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

const (
	testClusterURL = "http://localhost:8080/v2"

	providerConfig = `
provider "camundacluster" {
	url = "` + testClusterURL + `"
}
`

	// providerConfigShortConsistency shortens the provider's consistency timeout for steps
	// that expect a not-found error. In production a lookup that finds nothing keeps polling
	// for the full timeout, because the read side may simply not have caught up with a
	// create yet. Those steps already know the object does not exist, so waiting the whole
	// default only slows the suite.
	providerConfigShortConsistency = `
provider "camundacluster" {
	url                 = "` + testClusterURL + `"
	consistency_timeout = "5s"
}
`
)

// testAccProtoV6ProviderFactories is used to instantiate a provider during acceptance testing.
// The factory function is called for each Terraform CLI command to create a provider
// server that the CLI can connect to and interact with.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"camundacluster": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	// This endpoint doesn't require authentication
	statusEndpoint := testClusterURL + "/status"

	_, err := waitForConsistency(t.Context(), nil, "camunda cluster status endpoint", func() (int, bool, error) {
		resp, err := http.Get(statusEndpoint)
		if err != nil {
			// The cluster may not be reachable yet at all at this point in a
			// test run; treat that the same as a not-yet-ready status rather
			// than a hard error.
			return 0, false, nil
		}
		defer resp.Body.Close()
		return resp.StatusCode, resp.StatusCode == http.StatusNoContent, nil
	})

	if err != nil {
		t.Fatalf("pre-check failed: %s", err)
	}
}
