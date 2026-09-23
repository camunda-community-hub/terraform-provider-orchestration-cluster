# Assign an OIDC client, provisioned externally through the identity provider,
# to a Camunda cluster group. Members of the group inherit the group's
# authorizations, roles, and tenant assignments.
resource "camundacluster_client_group_assignment" "example" {
  group_id  = "my-group"
  client_id = "my-oidc-client-id"
}
