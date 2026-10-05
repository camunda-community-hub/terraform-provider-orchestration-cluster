package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var groupMemberClient = membershipDef{
	typeSuffix:  "group_member_client",
	ownerLabel:  "group",
	memberLabel: "client",
	assign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.AssignClientToGroupWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	unassign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.UnassignClientFromGroupWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	search: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string, body camunda.SearchQueryRequest) (membershipResponse, error) {
		r, err := c.SearchClientsForGroupWithResponse(ctx, ownerId, body)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	decodePage: decodeMembershipPage(func(r camunda.GroupClientResult) string { return r.ClientId }),
}

func NewGroupMemberClientResource() resource.Resource {
	return newMembershipResource(groupMemberClient)
}

// searchAllGroupClients reports whether memberId is assigned to ownerId, paging through all of the group's clients.
// Returns an error with message "not_found" if the group itself is not found.
func searchAllGroupClients(ctx context.Context, client *camunda.ClientWithResponses, ownerId, memberId string) (bool, error) {
	return groupMemberClient.contains(ctx, client, ownerId, memberId)
}
