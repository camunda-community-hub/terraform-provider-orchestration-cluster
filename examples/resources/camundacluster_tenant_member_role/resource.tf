resource "camundacluster_tenant" "acme" {
  tenant_id = "acme"
  name      = "Acme"
}

resource "camundacluster_role" "deployer" {
  role_id = "deployer"
  name    = "Deployer"
}

resource "camundacluster_tenant_member_role" "deployer" {
  tenant_id = camundacluster_tenant.acme.id
  role_id   = camundacluster_role.deployer.id
}
