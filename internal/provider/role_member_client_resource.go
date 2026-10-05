package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var roleMemberClient = membershipDef{
	typeSuffix:  "role_member_client",
	ownerLabel:  "role",
	memberLabel: "client",
	assign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.AssignRoleToClientWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	unassign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.UnassignRoleFromClientWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	search: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string, body camunda.SearchQueryRequest) (membershipResponse, error) {
		r, err := c.SearchClientsForRoleWithResponse(ctx, ownerId, body)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	decodePage: decodeMembershipPage(func(r camunda.RoleClientResult) string { return r.ClientId }),
}

func NewRoleMemberClientResource() resource.Resource {
	return newMembershipResource(roleMemberClient)
}

// searchAllRoleClients reports whether memberId is assigned to ownerId, paging through all of the role's clients.
// Returns an error with message "not_found" if the role itself is not found.
func searchAllRoleClients(ctx context.Context, client *camunda.ClientWithResponses, ownerId, memberId string) (bool, error) {
	return roleMemberClient.contains(ctx, client, ownerId, memberId)
}
