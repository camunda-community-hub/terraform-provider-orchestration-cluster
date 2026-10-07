package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
)

// mappingRuleMemberAccCases drives the acceptance and drift tests of the group and role
// mapping-rule member resources. config declares an owner named by ownerId and an assignment
// named "test" of memberId to it.
var mappingRuleMemberAccCases = []struct {
	name     string
	def      membershipDef
	ownerId  string
	memberId string
	config   string
}{
	{
		name:     "Group",
		def:      groupMemberMappingRule,
		ownerId:  "mappingrulemembergroup",
		memberId: "groupmembermappingrule",
		config: `
resource "camundacluster_group" "o" {
  group_id = "mappingrulemembergroup"
  name     = "mappingrulemembergroup"
}

resource "camundacluster_mapping_rule" "m" {
  mapping_rule_id = "groupmembermappingrule"
  claim_name      = "groups"
  claim_value     = "groupmembermappingrule"
  name            = "groupmembermappingrule"
}

resource "camundacluster_group_member_mapping_rule" "test" {
  group_id        = camundacluster_group.o.group_id
  mapping_rule_id = camundacluster_mapping_rule.m.mapping_rule_id
}
`,
	},
	{
		name:     "Role",
		def:      roleMemberMappingRule,
		ownerId:  "mappingrulememberrole",
		memberId: "rolemembermappingrule",
		config: `
resource "camundacluster_role" "o" {
  role_id = "mappingrulememberrole"
  name    = "mappingrulememberrole"
}

resource "camundacluster_mapping_rule" "m" {
  mapping_rule_id = "rolemembermappingrule"
  claim_name      = "groups"
  claim_value     = "rolemembermappingrule"
  name            = "rolemembermappingrule"
}

resource "camundacluster_role_member_mapping_rule" "test" {
  role_id         = camundacluster_role.o.role_id
  mapping_rule_id = camundacluster_mapping_rule.m.mapping_rule_id
}
`,
	},
}

func TestAccMappingRuleMemberResources(t *testing.T) {
	for _, tc := range mappingRuleMemberAccCases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: providerConfig + tc.config,
						Check:  checkMappingRuleMemberExistsInEngine(tc.def, tc.ownerId, tc.memberId),
					},
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

// TestAccMappingRuleMemberResources_driftDetection verifies that if the mapping rule is unassigned
// out-of-band, Read() removes the resource from state so Terraform plans to recreate it. It uses
// RefreshState rather than a Config step for the reasons given in
// TestAccTenantMemberClientResource_driftDetection.
func TestAccMappingRuleMemberResources_driftDetection(t *testing.T) {
	for _, tc := range mappingRuleMemberAccCases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: providerConfig + tc.config,
						Check:  checkMappingRuleMemberExistsInEngine(tc.def, tc.ownerId, tc.memberId),
					},
					{
						PreConfig:          unassignMappingRuleMemberOutOfBand(tc.def, tc.ownerId, tc.memberId),
						RefreshState:       true,
						ExpectNonEmptyPlan: true,
					},
				},
			})
		})
	}
}

func unassignMappingRuleMemberOutOfBand(def membershipDef, ownerId, memberId string) func() {
	return func() {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			panic(err)
		}
		if _, err := def.unassign(context.Background(), client, ownerId, memberId); err != nil {
			panic(err)
		}

		if _, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("%s %q %s %q unassignment", def.ownerLabel, ownerId, def.memberName(), memberId), func() (bool, bool, error) {
			found, err := def.contains(context.Background(), client, ownerId, memberId)
			if err != nil {
				return false, false, err
			}
			return !found, !found, nil
		}); err != nil {
			panic(err)
		}
	}
}

func checkMappingRuleMemberExistsInEngine(def membershipDef, ownerId, memberId string) resource.TestCheckFunc {
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
			if got := rs.Primary.Attributes[def.ownerAttr()]; got != ownerId {
				return fmt.Errorf("%s = %q in state, want %q", def.ownerAttr(), got, ownerId)
			}
			if got := rs.Primary.Attributes[def.memberAttr()]; got != memberId {
				return fmt.Errorf("%s = %q in state, want %q", def.memberAttr(), got, memberId)
			}
			_, err := waitForConsistency(context.Background(), nil, fmt.Sprintf("%s %q in %s %q in engine", def.memberName(), memberId, def.ownerLabel, ownerId), func() (bool, bool, error) {
				found, err := def.contains(context.Background(), client, ownerId, memberId)
				if err != nil {
					return false, false, err
				}
				return found, found, nil
			})
			if err != nil {
				return fmt.Errorf("%s %s not found in %s %s in engine: %w", def.memberName(), memberId, def.ownerLabel, ownerId, err)
			}
			return nil
		}
		return fmt.Errorf("%s resource not found in Terraform state", rt)
	}
}
