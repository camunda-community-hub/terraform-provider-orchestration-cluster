package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
)

var tenantMemberMappingRule = membershipDef{
	typeSuffix:  "tenant_member_mapping_rule",
	ownerLabel:  "tenant",
	memberLabel: "mapping_rule",
	assign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.AssignMappingRuleToTenantWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	unassign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.UnassignMappingRuleFromTenantWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	search: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string, body camunda.SearchQueryRequest) (membershipResponse, error) {
		r, err := c.SearchMappingRulesForTenantWithResponse(ctx, ownerId, body)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	decodePage: decodeMembershipPage(func(r camunda.MappingRuleResult) string { return r.MappingRuleId }),
}

func NewTenantMemberMappingRuleResource() resource.Resource {
	return newMembershipResource(tenantMemberMappingRule)
}
