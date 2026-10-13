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
3. **Signing key:** the release signing key is issued from the central key source, not generated per repository. Do not create a key for this repository, and keep the private key out of the repository.
4. **Repository secrets:** set `GPG_PRIVATE_KEY` (the armored private key) and `PASSPHRASE` (its passphrase) from the key you were issued. `release.yml` imports the key and passes its fingerprint to GoReleaser as `GPG_FINGERPRINT`.
5. **Registry:** the matching public key has to be registered for the `camunda-community-hub` namespace on [registry.terraform.io](https://registry.terraform.io), which needs an organization admin. The provider is published once through the registry's Publish > Provider page; after that, new releases are ingested automatically.

### Cutting a release

1. In `CHANGELOG.md`, add the entry for the next version with its release date, for example `## 0.3.0 (October 20, 2026)`, and merge it to `main`. The release archives ship `CHANGELOG.md`, and the workflow refuses to release a version without a dated entry.
2. Go to Actions > Release > Run workflow, keep the branch `main`, and choose the release type: `patch`, `minor` or `major`. The workflow bumps the latest `v*` tag accordingly (for example `v0.2.0` becomes `v0.2.1`, `v0.3.0` or `v1.0.0`). While the project is `0.x`, breaking changes go into a `minor` release. Tick `dry_run` first if you want to see the computed version and have the preconditions checked without releasing anything.
3. The workflow then runs the full CI on the commit, creates the annotated tag, and builds, signs and publishes the release. It stops at the first failing step. If a flaky test fails the CI, run the workflow again.
4. Check that the `Release` workflow succeeded, that the GitHub release has the signed checksums and the `*_manifest.json`, and that `terraform init` downloads the new version from the registry.

The steps of the workflow (checking the branch, computing the version, checking the changelog, creating the tag) are a small Go program in `.github/scripts/release`, not inline shell. It has unit tests, including tests against scratch git repositories, which CI runs with `go test ./.github/scripts/...` (a plain `go test ./...` skips directories that start with a dot). To see what version a release type would produce, run it from the repository root; it prints the result and does not change anything:

```
RELEASE_TYPE=minor go run ./.github/scripts/release next-version
```

Pushing a `v*` tag by hand still works and goes straight to building and publishing, without the CI run and the checks above.

The tag created by the workflow is pushed with the default `GITHUB_TOKEN`, which does not start other workflow runs, so the tag does not trigger a second release. Keep it that way: using a personal access token for the push would. The registry webhook listens for the GitHub release that GoReleaser creates, not for tags, so it is not affected.

Optionally dry-run the build locally (no signing, nothing published; GoReleaser is run with `go run`, so nothing is added to `go.mod`):
```
go run github.com/goreleaser/goreleaser/v2@v2 release --snapshot --clean --skip=sign,publish
```
The binaries and archives land in `dist/`, which is git-ignored.
