resource "orchestration_cluster_tenant" "acme" {
  tenant_id = "acme"
  name      = "Acme"
}

resource "orchestration_cluster_user" "alice" {
  username = "alice"
  name     = "Alice Smith"
  email    = "alice@example.com"
  password = var.alice_password
}

resource "orchestration_cluster_tenant_member_user" "alice" {
  tenant_id = orchestration_cluster_tenant.acme.id
  user_id   = orchestration_cluster_user.alice.id
}
