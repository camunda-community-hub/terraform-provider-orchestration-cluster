resource "orchestration_cluster_tenant" "acme" {
  tenant_id = "acme"
  name      = "Acme"
}

resource "orchestration_cluster_tenant_member_client" "ci" {
  tenant_id = orchestration_cluster_tenant.acme.id
  client_id = "ci-pipeline"
}
