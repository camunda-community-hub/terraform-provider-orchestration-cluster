# Assign an OIDC client, provisioned externally through the identity provider,
# to a Camunda cluster tenant.
resource "camundacluster_client_tenant_assignment" "example" {
  tenant_id = "my-tenant"
  client_id = "my-oidc-client-id"
}
