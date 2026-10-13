resource "orchestration_cluster_role" "ops" {
  role_id = "ops"
  name    = "Ops"
}

resource "orchestration_cluster_mapping_rule" "admin" {
  mapping_rule_id = "admin-mapping"
  claim_name      = "groups"
  claim_value     = "admin"
  name            = "Admin Mapping"
}

resource "orchestration_cluster_role_member_mapping_rule" "admin" {
  role_id         = orchestration_cluster_role.ops.role_id
  mapping_rule_id = orchestration_cluster_mapping_rule.admin.mapping_rule_id
}
