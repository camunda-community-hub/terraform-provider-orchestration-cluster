resource "camundacluster_user" "alice" {
  username = "alice"
  name     = "Alice Smith"
  email    = "alice@example.com"
  password = var.alice_password
}
