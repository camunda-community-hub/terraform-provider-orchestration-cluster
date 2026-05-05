# Terraform Provider: Camunda Orchestration Cluster

This provider manages the configuration of existing [Camunda](https://camunda.com) orchestration cluster instances. It does not provision or destroy clusters — it configures them.

## How it works

An orchestration cluster is represented as a `data` source. All resources (users, groups, roles, tenants, etc.) are scoped to a cluster data object, so a single Terraform state can manage configuration across multiple orchestration clusters simultaneously.

```hcl
data "orchestration-cluster_cluster" "production" {
  url           = "https://cluster.example.com"
  client_id     = var.client_id
  client_secret = var.client_secret
}

data "orchestration-cluster_cluster" "staging" {
  url           = "https://staging.cluster.example.com"
  client_id     = var.staging_client_id
  client_secret = var.staging_client_secret
}

resource "orchestration-cluster_user" "alice" {
  cluster  = data.orchestration-cluster_cluster.production.id
  username = "alice"
  email    = "alice@example.com"
}
```

## Resources

| Resource | Description |
|---|---|
| `orchestration-cluster_user` | Identity user |
| `orchestration-cluster_group` | Identity group |
| `orchestration-cluster_role` | Identity role |
| `orchestration-cluster_authorization` | Authorization assignment |
| `orchestration-cluster_client` | OAuth client |
| `orchestration-cluster_mapping_rule` | Identity mapping rule |
| `orchestration-cluster_tenant` | Tenant |
| `orchestration-cluster_cluster_variable` | Cluster variable |

## Data Sources

| Data Source | Description |
|---|---|
| `orchestration-cluster_cluster` | Existing orchestration cluster connection |

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- A running Camunda orchestration cluster with API access

## License

Apache 2.0
