resource "camundacluster_tenant" "example" {
  tenant_id   = "my-tenant"
  name        = "My Tenant"
  description = "A tenant managed by Terraform"
}
