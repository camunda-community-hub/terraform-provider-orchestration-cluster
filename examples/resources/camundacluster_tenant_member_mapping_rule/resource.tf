resource "camundacluster_tenant" "acme" {
  tenant_id = "acme"
  name      = "Acme"
}

resource "camundacluster_mapping_rule" "admin" {
  mapping_rule_id = "admin-mapping"
  claim_name      = "groups"
  claim_value     = "admin"
  name            = "Admin Mapping"
}

resource "camundacluster_tenant_member_mapping_rule" "admin" {
  tenant_id       = camundacluster_tenant.acme.id
  mapping_rule_id = camundacluster_mapping_rule.admin.mapping_rule_id
}
