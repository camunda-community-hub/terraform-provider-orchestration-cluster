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

## Developing

Add something similar to the following in your `~/.terraformrc` configuration file:

```hclprovider_installation {
provider_installation {
  dev_overrides {
    "scaffolding-framework" = "<your $GOPATH>/bin"
  }

  direct {}
}
```


---

(below is the original README from the https://github.com/hashicorp/terraform-plugin-framework repository

# Terraform Provider Scaffolding (Terraform Plugin Framework)

_This template repository is built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework). The template repository built on the [Terraform Plugin SDK](https://github.com/hashicorp/terraform-plugin-sdk) can be found at [terraform-provider-scaffolding](https://github.com/hashicorp/terraform-provider-scaffolding). See [Which SDK Should I Use?](https://developer.hashicorp.com/terraform/plugin/framework-benefits) in the Terraform documentation for additional information._

This repository is a *template* for a [Terraform](https://www.terraform.io) provider. It is intended as a starting point for creating Terraform providers, containing:

- A resource and a data source (`internal/provider/`),
- Examples (`examples/`) and generated documentation (`docs/`),
- Miscellaneous meta files.

These files contain boilerplate code that you will need to edit to create your own Terraform provider. Tutorials for creating Terraform providers can be found on the [HashiCorp Developer](https://developer.hashicorp.com/terraform/tutorials/providers-plugin-framework) platform. _Terraform Plugin Framework specific guides are titled accordingly._

Please see the [GitHub template repository documentation](https://help.github.com/en/github/creating-cloning-and-archiving-repositories/creating-a-repository-from-a-template) for how to create a new repository from this template on GitHub.

Once you've written your provider, you'll want to [publish it on the Terraform Registry](https://developer.hashicorp.com/terraform/registry/providers/publishing) so that others can use it.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.24

## Building the Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Using the Provider

Fill this in for each provider

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `make generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```
