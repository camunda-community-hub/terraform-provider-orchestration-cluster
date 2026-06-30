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
	testClusterURL = "http://localhost:8080/v2"

	providerConfig = `
provider "camundacluster" {
	url = "` + testClusterURL + `"
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
	statusEndpoint := testClusterURL + "/status"
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
