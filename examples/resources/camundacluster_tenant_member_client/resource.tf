resource "camundacluster_tenant" "acme" {
  tenant_id = "acme"
  name      = "Acme"
}

resource "camundacluster_tenant_member_client" "ci" {
  tenant_id = camundacluster_tenant.acme.id
  client_id = "ci-pipeline"
}
