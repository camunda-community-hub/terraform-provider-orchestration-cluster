package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

// tenantMemberAccCases drives the acceptance and drift tests of the tenant member resources
// that have no dedicated test file. config declares a tenant named by tenantId and an assignment
// named "test" of memberId to it.
var tenantMemberAccCases = []struct {
	name     string
	def      membershipDef
	tenantId string
	memberId string
	config   string
}{
	{
		name:     "User",
		def:      tenantMemberUser,
		tenantId: "usermembertenant",
		memberId: "tenantmemberuser1",
		config: `
resource "camundacluster_tenant" "t" {
  tenant_id = "usermembertenant"
  name      = "usermembertenant"
}

resource "camundacluster_user" "m" {
  username = "tenantmemberuser1"
  name     = "Tenant Member User"
  email    = "tenantmemberuser1@example.com"
  password = "testpass123"
}

resource "camundacluster_tenant_member_user" "test" {
  tenant_id = camundacluster_tenant.t.tenant_id
  user_id   = camundacluster_user.m.username
}
`,
	},
	{
		name:     "Group",
		def:      tenantMemberGroup,
		tenantId: "groupmembertenant",
		memberId: "tenantmembergroup",
		config: `
resource "camundacluster_tenant" "t" {
  tenant_id = "groupmembertenant"
  name      = "groupmembertenant"
}

resource "camundacluster_group" "m" {
  group_id = "tenantmembergroup"
  name     = "tenantmembergroup"
}

resource "camundacluster_tenant_member_group" "test" {
  tenant_id = camundacluster_tenant.t.tenant_id
  group_id  = camundacluster_group.m.group_id
}
`,
	},
	{
		name:     "Role",
		def:      tenantMemberRole,
		tenantId: "rolemembertenant",
		memberId: "tenantmemberrole",
		config: `
resource "camundacluster_tenant" "t" {
  tenant_id = "rolemembertenant"
  name      = "rolemembertenant"
}

resource "camundacluster_role" "m" {
  role_id = "tenantmemberrole"
  name    = "tenantmemberrole"
}

resource "camundacluster_tenant_member_role" "test" {
  tenant_id = camundacluster_tenant.t.tenant_id
  role_id   = camundacluster_role.m.role_id
}
`,
	},
	{
		name:     "MappingRule",
		def:      tenantMemberMappingRule,
		tenantId: "mappingrulemembertenant",
		memberId: "tenantmembermappingrule",
		config: `
resource "camundacluster_tenant" "t" {
  tenant_id = "mappingrulemembertenant"
  name      = "mappingrulemembertenant"
}

resource "camundacluster_mapping_rule" "m" {
  mapping_rule_id = "tenantmembermappingrule"
  claim_name      = "groups"
  claim_value     = "tenantmembermappingrule"
  name            = "tenantmembermappingrule"
}

resource "camundacluster_tenant_member_mapping_rule" "test" {
  tenant_id       = camundacluster_tenant.t.tenant_id
  mapping_rule_id = camundacluster_mapping_rule.m.mapping_rule_id
}
`,
	},
}

func TestAccTenantMemberResources(t *testing.T) {
	for _, tc := range tenantMemberAccCases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					// Create and Read testing
					{
						Config: providerConfig + tc.config,
						Check:  checkTenantMemberExistsInEngine(tc.def, tc.tenantId, tc.memberId),
					},
					// ImportState testing
					{
						ResourceName:      "camundacluster_" + tc.def.typeSuffix + ".test",
						ImportState:       true,
						ImportStateVerify: true,
					},
				},
			})
		})
	}
}

// TestAccTenantMemberResources_driftDetection verifies that if the member is unassigned from the
// tenant out-of-band (outside Terraform), Read() detects the drift and removes the resource from
// state, causing Terraform to plan to recreate it. See TestAccTenantMemberClientResource_driftDetection
// for why this uses RefreshState rather than a Config step.
func TestAccTenantMemberResources_driftDetection(t *testing.T) {
	for _, tc := range tenantMemberAccCases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: providerConfig + tc.config,
						Check:  checkTenantMemberExistsInEngine(tc.def, tc.tenantId, tc.memberId),
					},
					{
						PreConfig:          unassignTenantMemberOutOfBand(tc.def, tc.tenantId, tc.memberId),
						RefreshState:       true,
						ExpectNonEmptyPlan: true,
					},
				},
			})
		})
	}
}

func unassignTenantMemberOutOfBand(def membershipDef, tenantId, memberId string) func() {
	return func() {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			panic(err)
		}
		if _, err := def.unassign(context.Background(), client, tenantId, memberId); err != nil {
			panic(err)
		}

		// The search index used by Read lags behind the unassignment; wait for the removal to
		// be visible so Terraform's refresh deterministically detects the drift.
		if _, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("tenant %q %s %q unassignment", tenantId, def.memberName(), memberId), func() (bool, bool, error) {
			found, err := def.contains(context.Background(), client, tenantId, memberId)
			if err != nil {
				return false, false, err
			}
			return !found, !found, nil
		}); err != nil {
			panic(err)
		}
	}
}

func checkTenantMemberExistsInEngine(def membershipDef, tenantId, memberId string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}
		rt := "camundacluster_" + def.typeSuffix
		for _, rs := range s.RootModule().Resources {
			if rs.Type != rt {
				continue
			}
			if got := rs.Primary.Attributes[def.ownerAttr()]; got != tenantId {
				return fmt.Errorf("%s = %q in state, want %q", def.ownerAttr(), got, tenantId)
			}
			if got := rs.Primary.Attributes[def.memberAttr()]; got != memberId {
				return fmt.Errorf("%s = %q in state, want %q", def.memberAttr(), got, memberId)
			}
			_, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("%s %q in tenant %q in engine", def.memberName(), memberId, tenantId), func() (bool, bool, error) {
				found, err := def.contains(context.Background(), client, tenantId, memberId)
				if err != nil {
					return false, false, err
				}
				return found, found, nil
			})
			if err != nil {
				return fmt.Errorf("%s %s not found in tenant %s in engine: %w", def.memberName(), memberId, tenantId, err)
			}
			return nil
		}
		return fmt.Errorf("%s resource not found in Terraform state", rt)
	}
}
