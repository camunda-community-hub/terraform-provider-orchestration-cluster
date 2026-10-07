## 0.1.0 (Unreleased)

First release. This is a `0.x` version: the resource and data source surface may still change
based on feedback before a 1.0 compatibility commitment.

NOTES:

* Targets the Camunda 8.9 orchestration cluster REST API.
* Eventually consistent reads (`GET` and `/search`) are handled by polling with a bounded timeout, 30 seconds by default, configurable with the provider's `consistency_timeout` attribute.

FEATURES:

* **Provider configuration:** cluster `url`, Basic Auth (`basic_auth`) or OIDC client credentials (`oidc`), and `consistency_timeout`.
* **New Resource:** `camundacluster_user`
* **New Resource:** `camundacluster_group`
* **New Resource:** `camundacluster_role`
* **New Resource:** `camundacluster_tenant`
* **New Resource:** `camundacluster_authorization`
* **New Resource:** `camundacluster_mapping_rule`
* **New Resource:** `camundacluster_cluster_variable` (global and tenant scoped)
* **New Resources:** `camundacluster_group_member_user`, `camundacluster_group_member_client`, `camundacluster_group_member_mapping_rule`
* **New Resources:** `camundacluster_role_member_user`, `camundacluster_role_member_client`, `camundacluster_role_member_group`, `camundacluster_role_member_mapping_rule`
* **New Resources:** `camundacluster_tenant_member_user`, `camundacluster_tenant_member_client`, `camundacluster_tenant_member_group`, `camundacluster_tenant_member_role`, `camundacluster_tenant_member_mapping_rule`
* **New Data Source:** `camundacluster_user`
* **New Data Source:** `camundacluster_group`
* **New Data Source:** `camundacluster_role`
* **New Data Source:** `camundacluster_tenant`
* **New Data Source:** `camundacluster_mapping_rule`
* **New Data Source:** `camundacluster_authorization`
* **New Data Source:** `camundacluster_cluster_topology`
