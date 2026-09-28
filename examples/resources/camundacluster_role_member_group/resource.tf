resource "camundacluster_role" "deployer" {
  role_id = "deployer"
  name    = "Deployer"
}

resource "camundacluster_group" "engineering" {
  group_id = "engineering"
  name     = "Engineering"
}

resource "camundacluster_role_member_group" "engineering_deployer" {
  role_id  = camundacluster_role.deployer.id
  group_id = camundacluster_group.engineering.id
}
