resource "camundacluster_group" "engineering" {
  group_id = "engineering"
  name     = "Engineering"
}

resource "camundacluster_user" "alice" {
  username = "alice"
  name     = "Alice Smith"
  email    = "alice@example.com"
  password = var.alice_password
}

resource "camundacluster_group_member_user" "alice_engineering" {
  group_id = camundacluster_group.engineering.id
  user_id  = camundacluster_user.alice.id
}
