# Terraform Provider: Camunda Cluster

This provider manages the configuration of existing [Camunda](https://camunda.com) orchestration cluster instances. It does not provision or destroy clusters — it configures them.

## How it works

The cluster connection is configured directly in the `provider` block. All resources are scoped to the provider, so a single Terraform workspace configures one cluster. Use [provider aliases](https://developer.hashicorp.com/terraform/language/providers/configuration#alias-multiple-provider-configurations) to manage multiple clusters from one state.

```hcl
provider "camundacluster" {
  url = "https://cluster.example.com/v2"

  oidc {
    login_url     = "https://login.example.com/oauth/token"
    audience      = "cluster.example.com"
    client_id     = var.client_id
    client_secret = var.client_secret
  }
}

resource "camundacluster_user" "alice" {
  username = "alice"
  email    = "alice@example.com"
}
```

Basic auth is also supported via a `basic_auth { username = ...; password = ... }` block instead of `oidc`.

### Multiple clusters

```hcl
provider "camundacluster" {
  alias = "production"
  url   = "https://cluster.example.com/v2"

  oidc {
    login_url     = "https://login.example.com/oauth/token"
    audience      = "cluster.example.com"
    client_id     = var.client_id
    client_secret = var.client_secret
  }
}

provider "camundacluster" {
  alias = "staging"
  url   = "https://staging.cluster.example.com/v2"

  oidc {
    login_url     = "https://login.example.com/oauth/token"
    audience      = "staging.cluster.example.com"
    client_id     = var.staging_client_id
    client_secret = var.staging_client_secret
  }
}

resource "camundacluster_user" "alice" {
  provider = camundacluster.production
  username = "alice"
  email    = "alice@example.com"
}
```

## Resources

| Resource | Description |
|---|---|
| `camundacluster_user` | Identity user |
| `camundacluster_group` | Identity group |
| `camundacluster_role` | Identity role |
| `camundacluster_authorization` | Authorization assignment |
| `camundacluster_client` | OAuth client |
| `camundacluster_mapping_rule` | Identity mapping rule |
| `camundacluster_tenant` | Tenant |
| `camundacluster_cluster_variable` | Cluster variable |

## Eventual consistency

Every mutating endpoint (`POST`/`PUT`/`DELETE`) of the orchestration cluster REST API is
strongly consistent: once a request succeeds, the change has been applied. However, **every
`GET` and `/search` endpoint is eventually consistent** — they read from a separate,
asynchronously-updated projection, not the mutation's own write path. A read performed
immediately after a successful create, update, or delete can therefore still return the
*pre-mutation* state for a short time:

- A `GET` right after a successful `POST` can 404 even though the resource was created.
- A `GET` right after a successful `PUT` can return the *old* field values.
- A `GET`/search right after a successful `DELETE` (or an unassignment) can still report the
  resource/membership as present.

This matters here because Terraform relies on exactly these reads: every resource's `Create`
and `Update` re-reads the resource afterward to populate computed state, and Terraform's own
post-apply refresh calls `Read` again right after `Create`/`Update` return. A naive
`Create`/`Update`/`Read` that trusts the first response back from a `GET`/search will
intermittently see stale data — surfacing as flaky `terraform apply` failures (a resource
Terraform just created appearing to not exist, an update appearing not to have taken effect,
or a just-deleted assignment still showing up on the next plan) that are hard to reproduce
locally and easy to mistake for a different bug.

**Every resource and data source in this provider must account for this** rather than issuing
a single `GET`/search and trusting the result immediately after a mutation. The shared
`waitForConsistency` helper (`internal/provider/consistency.go`) exists for exactly this: it
polls a read with backoff until it reflects the expected post-mutation state (or a bounded
timeout elapses, see below), and callers only need to supply the small closure describing what "expected"
means for their case. See `internal/provider/tenant_resource.go`'s `readTenantWithRetry`
(post-create) and `readTenantUntilConsistent` (post-update) for the canonical pattern to
follow when adding a new resource; several other resources in `internal/provider/` follow the
same shape for their own create/update/delete paths.

### Configuring the consistency timeout

Polling gives up after a bounded time, 30 seconds by default. Set `consistency_timeout` in the
`provider` block to change it, for example to allow more time on a slow cluster, or less to get
faster feedback when a data source looks up something that does not exist (such a lookup keeps
polling for the full timeout before reporting not-found):

```hcl
provider "camundacluster" {
  url                 = "https://cluster.example.com/v2"
  consistency_timeout = "2m"
}
```

The value is a Go duration string between `5s` and `10m`. It is a property of the provider
instance, so each provider alias (one per cluster) can use its own value. The polling delay
(1s) and minimum poll interval (2s) are fixed. Configuration through environment variables is
not supported yet and is tracked together with the other provider settings in
[#18](https://github.com/camunda/terraform-provider-orchestration-cluster/issues/18).

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- A running Camunda orchestration cluster with API access

## License

Apache 2.0

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

[pre-commit](https://pre-commit.com) is expected in your local setup. Install it (for example `brew install pre-commit`) and enable the hooks once per clone with `pre-commit install`. The hooks run `gofmt -s` on staged Go files and regenerate the docs (`make generate`) on every commit, and the same hooks run as a required CI check, so unformatted code or stale docs fail the pull request. `make generate` needs `terraform` on your `PATH`. See [DEV.md](DEV.md#pre-commit-hooks) for details.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```
