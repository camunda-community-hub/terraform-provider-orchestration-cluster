resource "orchestration_cluster_tenant" "acme" {
  tenant_id = "acme"
  name      = "Acme"
}

resource "orchestration_cluster_mapping_rule" "admin" {
  mapping_rule_id = "admin-mapping"
  claim_name      = "groups"
  claim_value     = "admin"
  name            = "Admin Mapping"
}

resource "orchestration_cluster_tenant_member_mapping_rule" "admin" {
  tenant_id       = orchestration_cluster_tenant.acme.id
  mapping_rule_id = orchestration_cluster_mapping_rule.admin.mapping_rule_id
}
