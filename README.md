# Terraform Provider: Camunda Cluster

This provider manages the configuration of existing [Camunda](https://camunda.com) orchestration cluster instances. It does not provision or destroy clusters — it configures them.

## How it works

The cluster connection is configured directly in the `provider` block. All resources are scoped to the provider, so a single Terraform workspace configures one cluster. Use [provider aliases](https://developer.hashicorp.com/terraform/language/providers/configuration#alias-multiple-provider-configurations) to manage multiple clusters from one state.

```hcl
provider "camunda_cluster" {
  url           = "https://cluster.example.com"
  client_id     = var.client_id
  client_secret = var.client_secret
}

resource "camunda_cluster_user" "alice" {
  username = "alice"
  email    = "alice@example.com"
}
```

### Multiple clusters

```hcl
provider "camunda_cluster" {
  alias         = "production"
  url           = "https://cluster.example.com"
  client_id     = var.client_id
  client_secret = var.client_secret
}

provider "camunda_cluster" {
  alias         = "staging"
  url           = "https://staging.cluster.example.com"
  client_id     = var.staging_client_id
  client_secret = var.staging_client_secret
}

resource "camunda_cluster_user" "alice" {
  provider = camunda_cluster.production
  username = "alice"
  email    = "alice@example.com"
}
```

## Resources

| Resource | Description |
|---|---|
| `camunda_cluster_user` | Identity user |
| `camunda_cluster_group` | Identity group |
| `camunda_cluster_role` | Identity role |
| `camunda_cluster_authorization` | Authorization assignment |
| `camunda_cluster_client` | OAuth client |
| `camunda_cluster_mapping_rule` | Identity mapping rule |
| `camunda_cluster_tenant` | Tenant |
| `camunda_cluster_cluster_variable` | Cluster variable |

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- A running Camunda orchestration cluster with API access

## License

Apache 2.0
