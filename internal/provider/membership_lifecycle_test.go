package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

// membershipLifecycleDefs lists every membership resource covered by the shared replacement,
// error-case and import tests in this file.
var membershipLifecycleDefs = []membershipDef{
	groupMemberUser,
	groupMemberClient,
	groupMemberMappingRule,
	roleMemberUser,
	roleMemberMappingRule,
	roleMemberClient,
	roleMemberGroup,
	tenantMemberUser,
	tenantMemberGroup,
	tenantMemberRole,
	tenantMemberMappingRule,
	tenantMemberClient,
}

// membershipTestIds returns the IDs used by the acceptance tests of def. They are unique per
// resource type so tests never collide on the shared cluster.
func membershipTestIds(def membershipDef) (ownerA, ownerB, memberA, memberB string) {
	prefix := strings.ReplaceAll(def.typeSuffix, "_", "")
	return prefix + "owna", prefix + "ownb", prefix + "mema", prefix + "memb"
}

// membershipKindHCL renders the Terraform resource that creates an object of the given kind
// (the owner or member label of a membershipDef) with the given ID. Clients have no resource of
// their own, so it returns "" and the second result is false.
func membershipKindHCL(kind, label, id string) (string, bool) {
	switch kind {
	case "user":
		return fmt.Sprintf(`
resource "camundacluster_user" %q {
  username = %q
  name     = %q
  email    = "%s@example.com"
  password = "testpass123"
}
`, label, id, id, id), true
	case "group":
		return fmt.Sprintf(`
resource "camundacluster_group" %q {
  group_id = %q
  name     = %q
}
`, label, id, id), true
	case "role":
		return fmt.Sprintf(`
resource "camundacluster_role" %q {
  role_id = %q
  name    = %q
}
`, label, id, id), true
	case "tenant":
		return fmt.Sprintf(`
resource "camundacluster_tenant" %q {
  tenant_id = %q
  name      = %q
}
`, label, id, id), true
	case "mapping_rule":
		return fmt.Sprintf(`
resource "camundacluster_mapping_rule" %q {
  mapping_rule_id = %q
  claim_name      = "groups"
  claim_value     = %q
  name            = %q
}
`, label, id, id, id), true
	case "client":
		return "", false
	}
	panic("unknown membership kind " + kind)
}

// membershipBlocks renders the resources that create the given owner and member objects of def
// and, if assignOwner is not empty, an assignment named "test" between assignOwner and
// assignMember. Labels are prefixed with the resource type so the blocks of several definitions
// can share one configuration.
func membershipBlocks(def membershipDef, owners, members []string, assignOwner, assignMember string) string {
	var b strings.Builder
	var deps []string
	for i, id := range owners {
		label := fmt.Sprintf("%s_owner%d", def.typeSuffix, i)
		if hcl, ok := membershipKindHCL(def.ownerLabel, label, id); ok {
			b.WriteString(hcl)
			deps = append(deps, fmt.Sprintf("camundacluster_%s.%s", def.ownerLabel, label))
		}
	}
	for i, id := range members {
		label := fmt.Sprintf("%s_member%d", def.typeSuffix, i)
		if hcl, ok := membershipKindHCL(def.memberLabel, label, id); ok {
			b.WriteString(hcl)
			deps = append(deps, fmt.Sprintf("camundacluster_%s.%s", def.memberLabel, label))
		}
	}
	if assignOwner != "" {
		fmt.Fprintf(&b, "\nresource \"camundacluster_%s\" \"test\" {\n  %s = %q\n  %s = %q\n", def.typeSuffix, def.ownerAttr(), assignOwner, def.memberAttr(), assignMember)
		if len(deps) > 0 {
			fmt.Fprintf(&b, "  depends_on = [%s]\n", strings.Join(deps, ", "))
		}
		b.WriteString("}\n")
	}
	return b.String()
}

// membershipConfig renders a configuration for a single definition, see membershipBlocks.
func membershipConfig(def membershipDef, owners, members []string, assignOwner, assignMember string) string {
	return providerConfig + membershipBlocks(def, owners, members, assignOwner, assignMember)
}

// membershipAllConfig renders one configuration covering every membershipLifecycleDefs entry.
// pick returns the owner and member IDs to assign for a definition, or "" to only create the
// objects. All resources are applied in one run, which keeps the acceptance suite fast.
func membershipAllConfig(pick func(def membershipDef) (assignOwner, assignMember string)) string {
	var b strings.Builder
	b.WriteString(providerConfig)
	for _, def := range membershipLifecycleDefs {
		ownerA, ownerB, memberA, memberB := membershipTestIds(def)
		assignOwner, assignMember := pick(def)
		b.WriteString(membershipBlocks(def, []string{ownerA, ownerB}, []string{memberA, memberB}, assignOwner, assignMember))
	}
	return b.String()
}

// membershipExpectAction returns a plan check expecting action on the assignment of every
// membershipLifecycleDefs entry.
func membershipExpectAction(action plancheck.ResourceActionType) tfresource.ConfigPlanChecks {
	var checks []plancheck.PlanCheck
	for _, def := range membershipLifecycleDefs {
		checks = append(checks, plancheck.ExpectResourceAction("camundacluster_"+def.typeSuffix+".test", action))
	}
	return tfresource.ConfigPlanChecks{PreApply: checks}
}

// TestAccMembershipResources_replacement verifies that changing either the owner ID or the
// member ID of an assignment plans a replacement rather than an in-place update. All membership
// resources are checked in one run.
func TestAccMembershipResources_replacement(t *testing.T) {
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []tfresource.TestStep{
			{Config: membershipAllConfig(func(def membershipDef) (string, string) {
				ownerA, _, memberA, _ := membershipTestIds(def)
				return ownerA, memberA
			})},
			{
				Config: membershipAllConfig(func(def membershipDef) (string, string) {
					_, ownerB, memberA, _ := membershipTestIds(def)
					return ownerB, memberA
				}),
				ConfigPlanChecks: membershipExpectAction(plancheck.ResourceActionReplace),
			},
			{
				Config: membershipAllConfig(func(def membershipDef) (string, string) {
					_, ownerB, _, memberB := membershipTestIds(def)
					return ownerB, memberB
				}),
				ConfigPlanChecks: membershipExpectAction(plancheck.ResourceActionReplace),
			},
		},
	})
}

// TestAccMembershipResources_nonexistentOwner verifies that assigning to an owner that does not
// exist fails instead of recording an assignment in state.
func TestAccMembershipResources_nonexistentOwner(t *testing.T) {
	for _, def := range membershipLifecycleDefs {
		t.Run(def.typeSuffix, func(t *testing.T) {
			_, _, memberA, _ := membershipTestIds(def)
			tfresource.ParallelTest(t, tfresource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []tfresource.TestStep{
					{
						Config:      membershipConfig(def, nil, []string{memberA}, "doesnotexist"+strings.ReplaceAll(def.typeSuffix, "_", ""), memberA),
						ExpectError: regexp.MustCompile(`Assignment Error`),
					},
				},
			})
		})
	}
}

// TestAccMembershipResources_adoptExisting verifies that creating an assignment that already
// exists (the API answers 409) adopts it instead of failing. All membership resources are checked
// in one run.
func TestAccMembershipResources_adoptExisting(t *testing.T) {
	assignA := func(def membershipDef) (string, string) {
		ownerA, _, memberA, _ := membershipTestIds(def)
		return ownerA, memberA
	}
	checks := []tfresource.TestCheckFunc{}
	for _, def := range membershipLifecycleDefs {
		ownerA, memberA := assignA(def)
		checks = append(checks, tfresource.TestCheckResourceAttr("camundacluster_"+def.typeSuffix+".test", "id", encodeMembershipId(ownerA, memberA)))
	}
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []tfresource.TestStep{
			{Config: membershipAllConfig(func(membershipDef) (string, string) { return "", "" })},
			{
				PreConfig: func() {
					for _, def := range membershipLifecycleDefs {
						ownerA, memberA := assignA(def)
						assignMembershipOutOfBand(t, def, ownerA, memberA)
					}
				},
				Config:           membershipAllConfig(assignA),
				ConfigPlanChecks: membershipExpectAction(plancheck.ResourceActionCreate),
				Check:            tfresource.ComposeAggregateTestCheckFunc(checks...),
			},
		},
	})
}

// assignMembershipOutOfBand creates the assignment through the API, bypassing Terraform, and
// waits until it is visible to searches so that Terraform's create deterministically sees a
// duplicate.
func assignMembershipOutOfBand(t *testing.T, def membershipDef, ownerId, memberId string) {
	t.Helper()
	ctx := t.Context()
	client, err := camunda.NewClientWithResponses(testClusterURL)
	if err != nil {
		t.Fatalf("creating client: %s", err)
	}
	resp, err := def.assign(ctx, client, ownerId, memberId)
	if err != nil {
		t.Fatalf("assigning %s %q to %s %q out-of-band: %s", def.memberName(), memberId, def.ownerLabel, ownerId, err)
	}
	if resp.Status < 200 || resp.Status > 299 {
		t.Fatalf("assigning %s %q to %s %q out-of-band: HTTP %d: %s", def.memberName(), memberId, def.ownerLabel, ownerId, resp.Status, resp.Body)
	}
	if _, err := waitForConsistency(ctx, nil, fmt.Sprintf("%s %q %s %q assignment", def.ownerLabel, ownerId, def.memberName(), memberId), func() (bool, bool, error) {
		found, err := def.contains(ctx, client, ownerId, memberId)
		return found, found, err
	}); err != nil {
		t.Fatalf("waiting for out-of-band assignment: %s", err)
	}
}

// TestMembershipResources_importInvalidId verifies that every membership resource rejects a
// malformed composite import ID with an error naming the expected format, and accepts a
// well-formed one.
func TestMembershipResources_importInvalidId(t *testing.T) {
	for _, def := range membershipLifecycleDefs {
		t.Run(def.typeSuffix, func(t *testing.T) {
			ctx := context.Background()
			r := &membershipResource{def: def}

			var schemaResp resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
			newState := func() tfsdk.State {
				return tfsdk.State{
					Schema: schemaResp.Schema,
					Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
				}
			}

			wantFormat := fmt.Sprintf("<%s>/<%s>", def.ownerAttr(), def.memberAttr())
			for _, bad := range []string{"", "onlyowner", "/member", "owner/", "%zz/member", "owner/%zz", "owner/member/extra", "owner//member"} {
				resp := resource.ImportStateResponse{State: newState()}
				r.ImportState(ctx, resource.ImportStateRequest{ID: bad}, &resp)
				if !resp.Diagnostics.HasError() {
					t.Errorf("import ID %q: want error, got none", bad)
					continue
				}
				d := resp.Diagnostics.Errors()[0]
				if d.Summary() != "Invalid Import ID" || !strings.Contains(d.Detail(), wantFormat) {
					t.Errorf("import ID %q: diagnostic = %q / %q, want Invalid Import ID mentioning %s", bad, d.Summary(), d.Detail(), wantFormat)
				}
			}

			resp := resource.ImportStateResponse{State: newState()}
			r.ImportState(ctx, resource.ImportStateRequest{ID: encodeMembershipId("own/er", "mem/ber")}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("well-formed import ID: unexpected error: %v", resp.Diagnostics)
			}
			var ownerId, memberId string
			resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root(def.ownerAttr()), &ownerId)...)
			resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root(def.memberAttr()), &memberId)...)
			if resp.Diagnostics.HasError() {
				t.Fatalf("reading imported state: %v", resp.Diagnostics)
			}
			if ownerId != "own/er" || memberId != "mem/ber" {
				t.Errorf("imported %s, %s = %q, %q; want own/er, mem/ber", def.ownerAttr(), def.memberAttr(), ownerId, memberId)
			}
		})
	}
}
