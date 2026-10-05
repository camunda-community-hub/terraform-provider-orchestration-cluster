package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var tenantMemberUser = membershipDef{
	typeSuffix:        "tenant_member_user",
	ownerLabel:        "tenant",
	memberLabel:       "user",
	memberDescription: "The username of the user.",
	assign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.AssignUserToTenantWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	unassign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.UnassignUserFromTenantWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	search: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string, body camunda.SearchQueryRequest) (membershipResponse, error) {
		r, err := c.SearchUsersForTenantWithResponse(ctx, ownerId, body)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	decodePage: decodeMembershipPage(func(r camunda.TenantUserResult) string { return r.Username }),
}

func NewTenantMemberUserResource() resource.Resource {
	return newMembershipResource(tenantMemberUser)
}
