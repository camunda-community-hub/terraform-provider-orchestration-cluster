resource "orchestration_cluster_role" "deployer" {
  role_id = "deployer"
  name    = "Deployer"
}

resource "orchestration_cluster_user" "alice" {
  username = "alice"
  name     = "Alice Smith"
  email    = "alice@example.com"
  password = var.alice_password
}

resource "orchestration_cluster_role_member_user" "alice_deployer" {
  role_id = orchestration_cluster_role.deployer.id
  user_id = orchestration_cluster_user.alice.id
}
