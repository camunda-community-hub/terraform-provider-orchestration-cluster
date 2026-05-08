package provider

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
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
	const expectedStatusCode = http.StatusNoContent

	err := retry.RetryContext(t.Context(), 30*time.Second, func() *retry.RetryError {
		resp, err := http.Get(statusEndpoint)
		if err != nil {
			return retry.RetryableError(fmt.Errorf("unable to query %s: %w", statusEndpoint, err))
		}

		got := resp.StatusCode
		if got == expectedStatusCode {
			return nil
		}
		return retry.RetryableError(fmt.Errorf("expected HTTP %d, got %d", expectedStatusCode, got))
	})

	if err != nil {
		t.Fatalf("pre-check failed: %s", err)
	}
}
