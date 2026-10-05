resource "camundacluster_role" "ops" {
  role_id = "ops"
  name    = "Ops"
}

resource "camundacluster_mapping_rule" "admin" {
  mapping_rule_id = "admin-mapping"
  claim_name      = "groups"
  claim_value     = "admin"
  name            = "Admin Mapping"
}

resource "camundacluster_role_member_mapping_rule" "admin" {
  role_id         = camundacluster_role.ops.role_id
  mapping_rule_id = camundacluster_mapping_rule.admin.mapping_rule_id
}
