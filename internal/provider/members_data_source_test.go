package provider

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestMemberSearchAll_collectsAllPages(t *testing.T) {
	client, requests := membershipSearchServer(t, []string{
		`{"items":[{"clientId":"a"}],"page":{"endCursor":"c1"}}`,
		`{"items":[{"clientId":"b"}],"page":{"endCursor":"c2"}}`,
		`{"items":[],"page":{"endCursor":"c3"}}`,
	})

	got, err := memberSearchOf(tenantMemberClient).all(context.Background(), client, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("all = %v, want [a b]", got)
	}
	if n := len(requests()); n != 3 {
		t.Errorf("got %d requests, want 3", n)
	}
}

func TestMemberSearchAll_emptyIsNotNil(t *testing.T) {
	client, _ := membershipSearchServer(t, []string{`{"items":[],"page":{}}`})

	got, err := memberSearchOf(tenantMemberClient).all(context.Background(), client, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("all = %#v, want an empty non-nil slice", got)
	}
}

// TestAccMembersDataSources covers the group, role and tenant members data sources: members
// of every type that the lookups report, an empty list for an owner without members, and the
// "Not Found" diagnostic for an owner that does not exist.
func TestAccMembersDataSources(t *testing.T) {
	const entities = `
resource "orchestration_cluster_group" "owner" {
  group_id = "membersds-group"
  name     = "membersds-group"
}

resource "orchestration_cluster_group" "empty" {
  group_id = "membersds-empty-group"
  name     = "membersds-empty-group"
}

resource "orchestration_cluster_role" "owner" {
  role_id = "membersds-role"
  name    = "membersds-role"
}

resource "orchestration_cluster_tenant" "owner" {
  tenant_id = "membersds-tenant"
  name      = "membersds-tenant"
}

resource "orchestration_cluster_user" "member" {
  username = "membersds-user"
  name     = "Members DS User"
  email    = "membersds-user@example.com"
  password = "testpass123"
}

resource "orchestration_cluster_group_member_user" "group_user" {
  group_id = orchestration_cluster_group.owner.group_id
  user_id  = orchestration_cluster_user.member.username
}

resource "orchestration_cluster_group_member_client" "group_client" {
  group_id  = orchestration_cluster_group.owner.group_id
  client_id = "membersds-client"
}

resource "orchestration_cluster_role_member_group" "role_group" {
  role_id  = orchestration_cluster_role.owner.role_id
  group_id = orchestration_cluster_group.owner.group_id
}

resource "orchestration_cluster_tenant_member_role" "tenant_role" {
  tenant_id = orchestration_cluster_tenant.owner.tenant_id
  role_id   = orchestration_cluster_role.owner.role_id
}
`
	const lookups = `
data "orchestration_cluster_group_members" "owner" {
  group_id   = orchestration_cluster_group.owner.group_id
  depends_on = [orchestration_cluster_group_member_user.group_user, orchestration_cluster_group_member_client.group_client, orchestration_cluster_role_member_group.role_group]
}

data "orchestration_cluster_group_members" "empty" {
  group_id = orchestration_cluster_group.empty.group_id
}

data "orchestration_cluster_role_members" "owner" {
  role_id    = orchestration_cluster_role.owner.role_id
  depends_on = [orchestration_cluster_role_member_group.role_group]
}

data "orchestration_cluster_tenant_members" "owner" {
  tenant_id  = orchestration_cluster_tenant.owner.tenant_id
  depends_on = [orchestration_cluster_tenant_member_role.tenant_role]
}
`
	set := func(values ...string) knownvalue.Check {
		checks := make([]knownvalue.Check, len(values))
		for i, v := range values {
			checks[i] = knownvalue.StringExact(v)
		}
		return knownvalue.SetExact(checks)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + entities + lookups,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.orchestration_cluster_group_members.owner", tfjsonpath.New("id"), knownvalue.StringExact("membersds-group")),
					statecheck.ExpectKnownValue("data.orchestration_cluster_group_members.owner", tfjsonpath.New("user_ids"), set("membersds-user")),
					statecheck.ExpectKnownValue("data.orchestration_cluster_group_members.owner", tfjsonpath.New("client_ids"), set("membersds-client")),
					statecheck.ExpectKnownValue("data.orchestration_cluster_group_members.owner", tfjsonpath.New("mapping_rule_ids"), set()),
					statecheck.ExpectKnownValue("data.orchestration_cluster_group_members.owner", tfjsonpath.New("role_ids"), set("membersds-role")),
					// An owner without members is a valid result, not an error.
					statecheck.ExpectKnownValue("data.orchestration_cluster_group_members.empty", tfjsonpath.New("user_ids"), set()),
					statecheck.ExpectKnownValue("data.orchestration_cluster_group_members.empty", tfjsonpath.New("role_ids"), set()),
					statecheck.ExpectKnownValue("data.orchestration_cluster_role_members.owner", tfjsonpath.New("group_ids"), set("membersds-group")),
					statecheck.ExpectKnownValue("data.orchestration_cluster_role_members.owner", tfjsonpath.New("user_ids"), set()),
					statecheck.ExpectKnownValue("data.orchestration_cluster_tenant_members.owner", tfjsonpath.New("role_ids"), set("membersds-role")),
					statecheck.ExpectKnownValue("data.orchestration_cluster_tenant_members.owner", tfjsonpath.New("group_ids"), set()),
				},
			},
			// A missing owner is an error.
			{
				Config: providerConfigShortConsistency + `
data "orchestration_cluster_group_members" "missing" {
  group_id = "membersds-does-not-exist"
}
`,
				ExpectError: regexp.MustCompile(`No group found with ID`),
			},
			{
				Config: providerConfigShortConsistency + `
data "orchestration_cluster_role_members" "missing" {
  role_id = "membersds-does-not-exist"
}
`,
				ExpectError: regexp.MustCompile(`No role found with ID`),
			},
			{
				Config: providerConfigShortConsistency + `
data "orchestration_cluster_tenant_members" "missing" {
  tenant_id = "membersds-does-not-exist"
}
`,
				ExpectError: regexp.MustCompile(`No tenant found with ID`),
			},
		},
	})
}
