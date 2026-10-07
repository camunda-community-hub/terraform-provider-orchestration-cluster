package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
)

var tenantMemberClient = membershipDef{
	typeSuffix:  "tenant_member_client",
	ownerLabel:  "tenant",
	memberLabel: "client",
	assign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.AssignClientToTenantWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	unassign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.UnassignClientFromTenantWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	search: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string, body camunda.SearchQueryRequest) (membershipResponse, error) {
		r, err := c.SearchClientsForTenantWithResponse(ctx, ownerId, body)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	decodePage: decodeMembershipPage(func(r camunda.TenantClientResult) string { return r.ClientId }),
}

func NewTenantMemberClientResource() resource.Resource {
	return newMembershipResource(tenantMemberClient)
}

// searchAllTenantClients reports whether memberId is assigned to ownerId, paging through all of the tenant's clients.
// Returns an error with message "not_found" if the tenant itself is not found.
func searchAllTenantClients(ctx context.Context, client *camunda.ClientWithResponses, ownerId, memberId string) (bool, error) {
	return tenantMemberClient.contains(ctx, client, ownerId, memberId)
}
