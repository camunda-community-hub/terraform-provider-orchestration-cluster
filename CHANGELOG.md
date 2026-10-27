## 0.3.1 (October 27, 2026)

BUG FIXES:

* `orchestration_cluster_authorization`: the `resource_type` documentation now lists `BACKUP`, `EXPORTER` and `SECRET`. They were already accepted by the provider but missing from the docs.

## 0.3.0 (October 27, 2026)

BREAKING CHANGES:

* The provider now requires Camunda 8.10 or later and is built against the 8.10 Orchestration Cluster API. Camunda 8.9 and earlier are no longer supported.

ENHANCEMENTS:

* `orchestration_cluster_cluster_variable` (resource and data source) supports the 8.10 attributes `kind` (`JSON` or `SECRET_REFERENCE`; changing it replaces the variable) and `metadata`.
* `orchestration_cluster_cluster_topology` now also exposes `last_completed_change_id` and the `brokers` with their `partitions` (including the broker ID and partition state added in 8.10).
* **New Data Sources:** `orchestration_cluster_group_members`, `orchestration_cluster_role_members` and `orchestration_cluster_tenant_members` list the members of a group, role or tenant by member type, including members Terraform does not manage.
* **New Data Source:** `orchestration_cluster_cluster_variable` looks up a cluster variable by name and optional tenant.
* Every resource and data source page now links to the matching Camunda documentation page.
* Descriptions now say "inside a Camunda cluster" (for example "A group inside a Camunda cluster") so that `orchestration_cluster_group` and the other entities are not read as a group of clusters. The README no longer labels entities as "Identity", as that component is now called Admin.

## 0.2.0 (October 13, 2026)

BREAKING CHANGES:

* All resource and data source type names changed prefix from `camundacluster_` to `orchestration_cluster_` (for example `camundacluster_user` is now `orchestration_cluster_user`). Because Terraform derives the provider from the type name prefix, the preferred provider local name is now `orchestration` (another local name only works if every resource and data source sets `provider = ...` explicitly): declare it with `required_providers { orchestration = { source = "camunda-community-hub/orchestration-cluster" } }` and use `provider "orchestration" { ... }`.

NOTES:

* Users of 0.1.0 and 0.1.1 must update their configuration (resource and data source types, `provider` blocks, `provider = ...` references and `required_providers`) and migrate state. Tested with Terraform 1.16.4 against v0.1.1 and a local build of this release (group, role, user, tenant, cluster variable, group membership and a data source):
  * Terraform records the provider by its registry source (`registry.terraform.io/camunda-community-hub/orchestration-cluster`), which is unchanged, so `terraform state replace-provider` is not needed.
  * With the new configuration and the old state, `terraform plan` and `apply` fail with "no schema available for camundacluster_..." until the old addresses are removed from state.
  * Working procedure: update the configuration, run `terraform state rm` for each old resource address, then `terraform import` each new resource address with the import ID documented for the resource. Old data source addresses also have to be removed from state (`state rm`), but they are not imported: Terraform re-reads them on the next plan. `state rm` does not touch the cluster. After this, `terraform plan` showed no changes for group, role, tenant and cluster variable.
  * Caveat: `password` of `orchestration_cluster_user` is not imported, so the first plan wants an in-place update of the user, and a membership that references the user is replaced (destroy and create). Applying that worked and left no further changes. Adding `lifecycle { ignore_changes = [password] }` to the user resource avoided both changes in a plan test.
  * Does not work: `terraform state mv` ("resource types don't match") and a `moved` block ("target resource implementation does not include move resource state support", as the provider has no `MoveState`). Alternatively recreate the resources.

## 0.1.1 (October 13, 2026)

NOTES:

* First version published to the Terraform Registry. It is signed with the release key registered for the `camunda-community-hub` namespace.
* Identical to 0.1.0 apart from a CI dependency update (`goreleaser-action` v7.3.0). There are no provider changes.

## 0.1.0 (October 8, 2026)

First release. This is a `0.x` version: the resource and data source surface may still change
based on feedback before a 1.0 compatibility commitment.

NOTES:

* Tagged and released on GitHub only. The Terraform Registry could not verify the signature of this version's checksum file, so 0.1.0 is not available from the registry. Use 0.1.1 or later.
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
