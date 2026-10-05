# Look up a tenant by its tenant ID.
data "camundacluster_tenant" "by_id" {
  tenant_id = "my-tenant"
}

# Or look up a tenant by its (unique) name.
data "camundacluster_tenant" "by_name" {
  name = "My Tenant"
}
