resource "camundacluster_mapping_rule" "admin" {
  claim_name  = "groups"
  claim_value = "admin"
  name        = "Admin Mapping"
}
