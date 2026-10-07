package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
)

var groupMemberUser = membershipDef{
	typeSuffix:        "group_member_user",
	ownerLabel:        "group",
	memberLabel:       "user",
	memberDescription: "The username of the user.",
	assign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.AssignUserToGroupWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	unassign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.UnassignUserFromGroupWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	search: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string, body camunda.SearchQueryRequest) (membershipResponse, error) {
		r, err := c.SearchUsersForGroupWithResponse(ctx, ownerId, body)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	decodePage: decodeMembershipPage(func(r camunda.GroupUserResult) string { return r.Username }),
}

func NewGroupMemberUserResource() resource.Resource {
	return newMembershipResource(groupMemberUser)
}

// searchAllGroupUsers reports whether memberId is assigned to ownerId, paging through all of the group's users.
// Returns an error with message "not_found" if the group itself is not found.
func searchAllGroupUsers(ctx context.Context, client *camunda.ClientWithResponses, ownerId, memberId string) (bool, error) {
	return groupMemberUser.contains(ctx, client, ownerId, memberId)
}
