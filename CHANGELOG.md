## 0.2.0 (Unreleased)

BREAKING CHANGES:

* All resource and data source type names changed prefix from `camundacluster_` to `orchestration_cluster_` (for example `camundacluster_user` is now `orchestration_cluster_user`). Because Terraform derives the provider from the type name prefix, the provider local name is now `orchestration`: declare it with `required_providers { orchestration = { source = "camunda-community-hub/orchestration-cluster" } }` and use `provider "orchestration" { ... }`.

NOTES:

* Users of 0.1.0 and 0.1.1 must update their configuration (resource and data source types, `provider` blocks, `provider = ...` references and `required_providers`). Existing state must be migrated, for example with `terraform state mv` for each resource (state records the provider by its registry source, which is unchanged, so `terraform state replace-provider` should not be needed), or the resources must be recreated. This migration path has not been tested.

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
