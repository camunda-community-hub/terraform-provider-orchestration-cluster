resource "camundacluster_role" "deployer" {
  role_id = "deployer"
  name    = "Deployer"
}

resource "camundacluster_user" "alice" {
  username = "alice"
  name     = "Alice Smith"
  email    = "alice@example.com"
  password = var.alice_password
}

resource "camundacluster_role_member_user" "alice_deployer" {
  role_id = camundacluster_role.deployer.id
  user_id = camundacluster_user.alice.id
}
