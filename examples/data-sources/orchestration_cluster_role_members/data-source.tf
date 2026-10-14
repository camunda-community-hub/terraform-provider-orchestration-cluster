# Also works for built-in roles that Terraform does not manage.
data "orchestration_cluster_role_members" "example" {
  role_id = "admin"
}

output "admin_group_ids" {
  value = data.orchestration_cluster_role_members.example.group_ids
}
