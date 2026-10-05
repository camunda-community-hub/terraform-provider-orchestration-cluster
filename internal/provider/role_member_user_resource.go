package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var roleMemberUser = membershipDef{
	typeSuffix:        "role_member_user",
	ownerLabel:        "role",
	memberLabel:       "user",
	memberDescription: "The username of the user.",
	assign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.AssignRoleToUserWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	unassign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.UnassignRoleFromUserWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	search: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string, body camunda.SearchQueryRequest) (membershipResponse, error) {
		r, err := c.SearchUsersForRoleWithResponse(ctx, ownerId, body)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	decodePage: decodeMembershipPage(func(r camunda.RoleUserResult) string { return string(r.Username) }),
}

func NewRoleMemberUserResource() resource.Resource {
	return newMembershipResource(roleMemberUser)
}

// searchAllRoleUsers reports whether memberId is assigned to ownerId, paging through all of the role's users.
// Returns an error with message "not_found" if the role itself is not found.
func searchAllRoleUsers(ctx context.Context, client *camunda.ClientWithResponses, ownerId, memberId string) (bool, error) {
	return roleMemberUser.contains(ctx, client, ownerId, memberId)
}
