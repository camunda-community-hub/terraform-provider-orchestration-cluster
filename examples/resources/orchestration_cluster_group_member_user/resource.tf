resource "orchestration_cluster_group" "engineering" {
  group_id = "engineering"
  name     = "Engineering"
}

resource "orchestration_cluster_user" "alice" {
  username = "alice"
  name     = "Alice Smith"
  email    = "alice@example.com"
  password = var.alice_password
}

resource "orchestration_cluster_group_member_user" "alice_engineering" {
  group_id = orchestration_cluster_group.engineering.id
  user_id  = orchestration_cluster_user.alice.id
}
