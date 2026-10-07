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
         "registry.terraform.io/camunda-community-hub/orchestration-cluster" = "<output of go env GOBIN>"
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

## Releasing

Releases are cut by pushing a `v*` tag. `.github/workflows/release.yml` then runs GoReleaser (`.goreleaser.yml`), which builds the provider for all platforms, signs the checksums with GPG, and publishes a GitHub release with the registry manifest attached. The Terraform Registry picks the release up through its webhook.

The provider is published as `camunda-community-hub/orchestration-cluster`
(`source = "camunda-community-hub/orchestration-cluster"` in `required_providers`). The registry derives that name from the repository name (`terraform-provider-<name>`) and the GitHub organization.

### One-time setup (repository or organization admin)

1. **License:** `LICENSE` (Apache 2.0) must be present on the default branch.
2. **Visibility:** the repository is public, which the registry requires: the public registry only lists public GitHub repositories. Keep it public.
3. **Signing key:** generate a dedicated GPG key for release signing and keep the private key out of the repository.
   ```
   gpg --full-generate-key
   gpg --armor --export-secret-keys <key-id>   # value for GPG_PRIVATE_KEY
   gpg --armor --export <key-id>               # public key for the registry
   ```
4. **Repository secrets:** add `GPG_PRIVATE_KEY` (the armored private key) and `PASSPHRASE` (its passphrase). `release.yml` imports the key and passes its fingerprint to GoReleaser as `GPG_FINGERPRINT`.
5. **Registry:** sign in to [registry.terraform.io](https://registry.terraform.io) with GitHub, authorize the `camunda-community-hub` organization, add the armored **public** key under the organization's signing keys, and publish this repository as a provider. After that, new `v*` releases are ingested automatically.

### Cutting a release

1. Make sure CI is green on `main` at the commit to be tagged.
2. In `CHANGELOG.md`, replace `(Unreleased)` on the version heading with the release date and review the entries.
3. Optionally dry-run the build locally (no signing, nothing published; GoReleaser is run with `go run`, so nothing is added to `go.mod`):
   ```
   go run github.com/goreleaser/goreleaser/v2@v2 release --snapshot --clean --skip=sign,publish
   ```
   The binaries and archives land in `dist/`, which is git-ignored.
4. Tag and push, for example `git tag v0.1.0 && git push origin v0.1.0`.
5. Check that the `Release` workflow succeeded, that the GitHub release has the signed checksums and the `*_manifest.json`, and that `terraform init` downloads the new version from the registry.
