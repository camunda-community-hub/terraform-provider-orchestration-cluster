resource "orchestration_cluster_tenant" "acme" {
  tenant_id = "acme"
  name      = "Acme"
}

resource "orchestration_cluster_role" "deployer" {
  role_id = "deployer"
  name    = "Deployer"
}

resource "orchestration_cluster_tenant_member_role" "deployer" {
  tenant_id = orchestration_cluster_tenant.acme.id
  role_id   = orchestration_cluster_role.deployer.id
}
