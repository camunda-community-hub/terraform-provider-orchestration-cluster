# Developing

## Regenerating the API client

```
make -f Makefile.camunda-client CAMUNDA_REPO=<path-to-camunda-repo> generate
```

## Building the provider

make build
```

## Using the provider

> [!NOTE]
> Introduction documentation on writing Terraform providers: https://developer.hashicorp.com/terraform/tutorials/providers-plugin-framework/providers-plugin-framework-provider#prepare-terraform-for-local-provider-install


1. Find the `GOBIN` environment variable value:
   ```
   go env GOBIN
   ```
2. Configure the Terraform configuration file to instruct Terraform where to search the local provider binary in:
   ```hcl
   provider_installation {

     dev_overrides {
         "camunda.com/camunda/camunda-cluster" = "<output of go env GOBIN>"
     }

     # Keep this for normal providers.
     direct {}
   }
   ```
3. Install the provider:
   ```
   make install
   ```


## Pre-commit hooks

The repository ships a [pre-commit](https://pre-commit.com) configuration so unformatted code and stale generated docs are caught before CI. Install pre-commit (for example `brew install pre-commit`), then enable the hooks once per clone:

```
pre-commit install
```

On each commit it runs `gofmt -s` on staged Go files and `make generate-check` (`make generate`, about 5 seconds, then a check for new untracked files under `docs/` and `examples/`), even when no provider code changed. A hook that changes files, or generation that creates new untracked files, fails the commit: review the changes, `git add` them and commit again. The same hooks run in CI as the required `pre-commit` check. `make generate` needs `terraform` on the PATH. Run all hooks manually with `pre-commit run --all-files`.

## Testing the provider

Start unit test with:
```
make test
```

### Acceptance tests

Acceptance tests require a running Camunda cluster, start C8 with:

```
cd docker-compose
docker compose up -d
```

Run acceptance tests with:
```
make testacc
```
