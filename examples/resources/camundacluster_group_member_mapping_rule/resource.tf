resource "camundacluster_group" "ops" {
  group_id = "ops"
  name     = "Ops"
}

resource "camundacluster_mapping_rule" "admin" {
  mapping_rule_id = "admin-mapping"
  claim_name      = "groups"
  claim_value     = "admin"
  name            = "Admin Mapping"
}

resource "camundacluster_group_member_mapping_rule" "admin" {
  group_id        = camundacluster_group.ops.group_id
  mapping_rule_id = camundacluster_mapping_rule.admin.mapping_rule_id
}
