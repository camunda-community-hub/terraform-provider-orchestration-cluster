# HATER Review

## Verdict: PASS

Build is clean, `go vet` is clean, every resource has a Delete, every Read removes
state on 404, every assignment Read actually interrogates the engine, every test
hits the real cluster API in its CheckFunc, and the authorization key is stored as
a string. I went in expecting a dumpster fire and found something annoyingly close
to competent. It still has problems — but none of them rise to a blocker.

## Blockers

None. I checked. Twice. I'm as surprised as you are.

## Majors

- group_resource.go:84-88, role_resource.go:90-94 — The resource uses the human-facing
  `name` as the engine `groupId`/`roleId` (`GroupId: data.Name.ValueString()`). Display
  names are not identifiers. Camunda Identity IDs have a restricted character set;
  the moment someone writes `name = "Test Group 1"` (which your own test
  group_resource_test.go:24 does), you are shoving a string with spaces into an ID field.
  Either the engine rejects it (runtime failure the schema gave no warning about) or it
  silently accepts a garbage ID you can never type by hand again. There should be a
  separate, immutable `group_id`/`role_id` attribute (RequiresReplace), with `name`
  remaining the mutable display field — mirror how the membership resources already
  separate `group_id` from the member. Fix before this ships, or document loudly that
  `name` doubles as the ID and must be ID-shaped.

## Minors

- authorization_resource.go:170, group_resource.go:101-102, role_resource.go:111-117 —
  `apiResp.JSON201.X` is dereferenced after only checking `StatusCode() == 201`. The
  generated parser (client.gen.go:37187) only populates `JSON201` when the status is 201
  AND `Content-Type` contains "json". A 201 with an empty or non-JSON body leaves
  `JSON201 == nil` and you panic. In practice the 8.9 create endpoints always return a
  JSON body so this won't fire today, but it's a free nil-check you skipped. The same
  laziness sits in every `JSON200`/`JSON201` access. Add a `if apiResp.JSONxxx == nil`
  guard, or stop pretending HTTP responses are trustworthy.

- All eight `*_test.go` CheckFuncs hardcode `"http://localhost:8080/v2"` instead of
  deriving it from the `providerConfig` constant in provider_test.go:21. Now the URL
  lives in nine places. Change the port once and you'll be hunting for the nine you
  forgot. Hoist it into a shared `const testClusterURL` and reference it everywhere,
  including inside `providerConfig`.

- group_resource_test.go:56, role_resource_test.go:61 — `testAccGroupResourceConfig`
  and `testAccRoleResourceConfig` take a `groupId`/`roleId` parameter that is never
  used (`groupId string` then ignored). Dead parameter. Delete it or use it.

- group_member_user_resource.go:104 (and the four sibling assignment resources) —
  the composite ID is built with naive string concatenation `group_id + "/" + user_id`.
  No `ImportState` is implemented for any assignment resource, so this ID is never
  parsed back, which is why you got away with it — but it also means these resources
  cannot be imported at all. If import is out of scope, fine; if it isn't, that's a
  gap. Either way the `/`-delimited scheme breaks the instant an ID legitimately
  contains a slash.

- Provider config comment block (provider_test.go:15-23) still references "HashiCups"
  and "HASHICUPS_ environment variables" — leftover scaffolding from the tutorial you
  copied. Scrub it. Comments that lie about the system are worse than no comments.

- The `Update` method on every assignment resource is an empty body with a comment.
  Correct, given everything is RequiresReplace — but an empty Update that silently
  does nothing is a trap for the next person. A one-line `tflog.Trace` or even leaving
  it truly unreachable would be clearer. Nitpick-adjacent; I'll allow it.

## Closing Remarks

Fine. It's... fine. The eight resources are near-identical copies of a sound template,
the assignment Reads do real membership checks instead of the usual "trust the state
and pray," and the tests don't lie to themselves by asserting Terraform state against
Terraform state. The `name`-as-ID conflation is the one thing that'll bite a real user,
and the nil-deref is the one thing that'll bite you at 3am when the engine returns an
empty 201. Fix those two and stop copy-pasting HashiCups boilerplate into your comments.
I dislike how little I have to complain about.
