resource "camundacluster_tenant" "acme" {
  tenant_id = "acme"
  name      = "Acme"
}

resource "camundacluster_group" "developers" {
  group_id = "developers"
  name     = "Developers"
}

resource "camundacluster_tenant_member_group" "developers" {
  tenant_id = camundacluster_tenant.acme.id
  group_id  = camundacluster_group.developers.id
}
