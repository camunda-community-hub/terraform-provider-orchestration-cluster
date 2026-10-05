resource "camundacluster_tenant" "acme" {
  tenant_id = "acme"
  name      = "Acme"
}

resource "camundacluster_user" "alice" {
  username = "alice"
  name     = "Alice Smith"
  email    = "alice@example.com"
  password = var.alice_password
}

resource "camundacluster_tenant_member_user" "alice" {
  tenant_id = camundacluster_tenant.acme.id
  user_id   = camundacluster_user.alice.id
}
