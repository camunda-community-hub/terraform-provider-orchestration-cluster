package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

const (
	// providerConfig is a shared configuration to combine with the actual
	// test configuration so the HashiCups client is properly configured.
	// It is also possible to use the HASHICUPS_ environment variables instead,
	// such as updating the Makefile and running the testing through that tool.
	providerConfig = `
provider "camundacluster" {
	url = "http://localhost:8080/v2"
}
`
)

// testAccProtoV6ProviderFactories is used to instantiate a provider during acceptance testing.
// The factory function is called for each Terraform CLI command to create a provider
// server that the CLI can connect to and interact with.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"camundacluster": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccProtoV6ProviderFactoriesWithEcho includes the echo provider alongside the camundacluster provider.
// It allows for testing assertions on data returned by an ephemeral resource during Open.
// The echoprovider is used to arrange tests by echoing ephemeral data into the Terraform state.
// This lets the data be referenced in test assertions with state checks.
//var testAccProtoV6ProviderFactoriesWithEcho = map[string]func() (tfprotov6.ProviderServer, error){
//    "camundacluster": providerserver.NewProtocol6WithError(New("test")()),
//    "echo":           echoprovider.NewProviderServer(),
//}

func testAccPreCheck(t *testing.T) {
	// You can add code here to run prior to any test case execution, for example assertions
	// about the appropriate environment variables being set are common to see in a pre-check
	// function.

	// This endpoint doesn't require authentication
	statusEndpoint := "http://localhost:8080/v2/status"

	_, err := waitForConsistency(t.Context(), "camunda cluster status endpoint", func() (int, bool, error) {
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
