# A global cluster variable.
data "orchestration_cluster_cluster_variable" "global" {
  name = "default-retries"
}

# A tenant-scoped cluster variable: set tenant_id.
data "orchestration_cluster_cluster_variable" "tenant" {
  name      = "greeting"
  tenant_id = "my-tenant"
}

# The value is a JSON document.
output "default_retries" {
  value = jsondecode(data.orchestration_cluster_cluster_variable.global.value)
}
