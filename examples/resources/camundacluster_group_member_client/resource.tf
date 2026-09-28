resource "camundacluster_group" "engineering" {
  group_id = "engineering"
  name     = "Engineering"
}

resource "camundacluster_group_member_client" "ci" {
  group_id  = camundacluster_group.engineering.id
  client_id = "ci-pipeline"
}
