resource "camundacluster_role" "deployer" {
  role_id = "deployer"
  name    = "Deployer"
}

resource "camundacluster_role_member_client" "ci" {
  role_id   = camundacluster_role.deployer.id
  client_id = "ci-pipeline"
}
