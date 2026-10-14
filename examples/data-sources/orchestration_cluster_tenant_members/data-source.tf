data "orchestration_cluster_tenant_members" "example" {
  tenant_id = "my-tenant"
}

output "tenant_client_ids" {
  value = data.orchestration_cluster_tenant_members.example.client_ids
}
