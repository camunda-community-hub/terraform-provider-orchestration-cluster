resource "orchestration_cluster_group" "ops" {
  group_id = "ops"
  name     = "Ops"
}

resource "orchestration_cluster_mapping_rule" "admin" {
  mapping_rule_id = "admin-mapping"
  claim_name      = "groups"
  claim_value     = "admin"
  name            = "Admin Mapping"
}

resource "orchestration_cluster_group_member_mapping_rule" "admin" {
  group_id        = orchestration_cluster_group.ops.group_id
  mapping_rule_id = orchestration_cluster_mapping_rule.admin.mapping_rule_id
}
