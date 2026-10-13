resource "orchestration_cluster_group" "engineering" {
  group_id = "engineering"
  name     = "Engineering"
}

resource "orchestration_cluster_group_member_client" "ci" {
  group_id  = orchestration_cluster_group.engineering.id
  client_id = "ci-pipeline"
}
