data "orchestration_cluster_group_members" "example" {
  group_id = "my-group"
}

output "group_user_ids" {
  value = data.orchestration_cluster_group_members.example.user_ids
}
