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
