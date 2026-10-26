# A global cluster variable. The value is a JSON document: use jsonencode().
resource "orchestration_cluster_cluster_variable" "global" {
  name  = "default-retries"
  value = jsonencode({ retries = 3 })
}

# A tenant-scoped cluster variable: set tenant_id.
resource "orchestration_cluster_cluster_variable" "tenant" {
  name      = "greeting"
  tenant_id = "my-tenant"
  value     = jsonencode("hello")
}

# A variable that references a secret, with metadata. The value holds the reference, not the secret.
resource "orchestration_cluster_cluster_variable" "secret_reference" {
  name  = "api-token"
  kind  = "SECRET_REFERENCE"
  value = jsonencode("camunda.secrets.API_TOKEN")

  metadata = {
    owner = "platform-team"
  }
}
