resource "orchestration_cluster_tenant" "acme" {
  tenant_id = "acme"
  name      = "Acme"
}

resource "orchestration_cluster_group" "developers" {
  group_id = "developers"
  name     = "Developers"
}

resource "orchestration_cluster_tenant_member_group" "developers" {
  tenant_id = orchestration_cluster_tenant.acme.id
  group_id  = orchestration_cluster_group.developers.id
}
