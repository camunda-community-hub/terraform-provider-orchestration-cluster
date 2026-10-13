resource "orchestration_cluster_role" "deployer" {
  role_id = "deployer"
  name    = "Deployer"
}

resource "orchestration_cluster_role_member_client" "ci" {
  role_id   = orchestration_cluster_role.deployer.id
  client_id = "ci-pipeline"
}
