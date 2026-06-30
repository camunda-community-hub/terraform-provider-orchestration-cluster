# HATER Review

## Verdict: PASS

The build is clean, `go vet` is silent, and every one of the eight resources and two data sources is wired up in `provider.go` with a matching file and a matching acceptance test that calls the live engine API. I went looking for orphan-creating Deletes, missing 404 handling, fake "Read" stubs that never touch the engine, and int64 IDs masquerading as keys. I found none of those. Annoying. The assignment Reads genuinely decode the engine search results and `RemoveResource` when the membership is gone. The authorization key is stored as a string, as it should be. Credit where it's grudgingly due.

What follows is what stops this from being spotless.

## Blockers
None. I checked twice.

## Majors

- `authorization_resource_test.go:48`, `group_resource_test.go:66`, `role_resource_test.go:79`, `group_member_user_resource_test.go:51`, `group_member_client_resource_test.go:44`, `role_member_user_resource_test.go:51`, `role_member_client_resource_test.go:45`, `role_member_group_resource_test.go:48` — Every engine-verification `CheckFunc` hardcodes `"http://localhost:8080/v2"` instead of deriving it from the `providerConfig` constant in `provider_test.go:19`. The URL now lives in nine places. Change the cluster endpoint once and you will be playing whack-a-mole, with the provider config and the checks silently disagreeing. Hoist the URL into one exported test constant (e.g. `const testClusterURL = "http://localhost:8080/v2"`), build `providerConfig` from it, and have every check call `camunda.NewClientWithResponses(testClusterURL)`.

## Minors

- `group_resource.go:84` / `role_resource.go:90` — `groupId`/`roleId` is set to the human display `name` verbatim (`data.Name.ValueString()`). The Camunda Identity API constrains these IDs to a restricted character set (no spaces). The test configs (`"Test Group 1"`, `"Test Role 1"`) only pass if the engine is lenient; the moment it enforces the documented pattern, both create calls 400. Conflating "display name" with "stable ID" is a design smell regardless — a rename should not require destroy/recreate. Consider a distinct, validated `group_id`/`role_id` attribute separate from `name`.

- `authorization_resource.go:107-157` and `:221-277` — The `if len(resourceIds) > 0 { ... } else { ... }` branches in Create and Update are near-identical, differing only in `ResourceId: resourceIds[0]` vs `ResourceId: "*"`. Collapse to one block that picks the id up front (`resourceId := "*"; if len(resourceIds) > 0 { resourceId = resourceIds[0] }`). Four near-duplicate blocks is four places to drift.

- `authorization_resource.go:38` — `resource_ids` is a `types.Set` but only `resourceIds[0]` is ever sent (`:138`, `:258`). A user supplying two resource IDs gets one silently honored and the rest discarded. Either enforce a single value in the schema or actually fan out across all of them. Lying about cardinality is worse than restricting it.

- `authorization_resource.go:170`, `group_resource.go:101`, `role_resource.go:111` — `apiResp.JSON201.X` is dereferenced after only checking `StatusCode() == 201`. The generated parser populates `JSON201` only on a 201 with a JSON content type; a 201 with an empty/non-JSON body would nil-panic. The 8.9 create endpoints always return a JSON body, so this does not fire today, but it is a free nil-guard that was skipped across every `JSON200`/`JSON201` access.

- `provider_test.go:14-23` and the stale commented-out `testAccProtoV6ProviderFactoriesWithEcho` block (`:37-40`) — Scaffolding cruft. The `providerConfig` doc comment still references "HashiCups" and "HASHICUPS_ environment variables", copy-pasted from the tutorial template. Also `provider.go:121` leaves a commented `// if data.Endpoint.IsNull() { ... }` line. Scrub all of it.

- Assignment resources have no `tflog.Trace` on create/delete, unlike `group_resource.go:104` and `role_resource.go:119`. Inconsistent — when one of these silently no-ops in the field you will wish you had the trace line.

## Closing Remarks

It pains me to write PASS. The structure is repetitive in the way generated-but-not-quite code always is, and the test suite hardcodes the same URL nine times like nobody expects the endpoint to ever move. But the things that actually cause data loss and silent drift — missing Deletes, Reads that never check the engine, swallowed 404s, int64 keys masquerading as IDs — are all handled correctly and uniformly across every resource. Fix the URL duplication before it grows a tenth copy, decide whether `name` is really your primary key, and stop shipping the tutorial's HashiCups comments. Then it's fine. ...Fine.
