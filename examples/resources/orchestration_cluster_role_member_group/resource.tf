resource "orchestration_cluster_role" "deployer" {
  role_id = "deployer"
  name    = "Deployer"
}

resource "orchestration_cluster_group" "engineering" {
  group_id = "engineering"
  name     = "Engineering"
}

resource "orchestration_cluster_role_member_group" "engineering_deployer" {
  role_id  = orchestration_cluster_role.deployer.id
  group_id = orchestration_cluster_group.engineering.id
}
