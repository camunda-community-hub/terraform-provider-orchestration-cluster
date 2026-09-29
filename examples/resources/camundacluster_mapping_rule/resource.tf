resource "camundacluster_mapping_rule" "admin" {
  mapping_rule_id = "admin-mapping"
  claim_name      = "groups"
  claim_value     = "admin"
  name            = "Admin Mapping"
}
