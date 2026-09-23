# Assign a role to an OIDC client provisioned externally through the identity
# provider. The client inherits the authorizations associated with the role.
resource "camundacluster_client_role_assignment" "example" {
  role_id   = "my-role"
  client_id = "my-oidc-client-id"
}
