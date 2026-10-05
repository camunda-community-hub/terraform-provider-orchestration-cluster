package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var roleMemberGroup = membershipDef{
	typeSuffix:  "role_member_group",
	ownerLabel:  "role",
	memberLabel: "group",
	assign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.AssignRoleToGroupWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	unassign: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error) {
		r, err := c.UnassignRoleFromGroupWithResponse(ctx, ownerId, memberId)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	search: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string, body camunda.SearchQueryRequest) (membershipResponse, error) {
		r, err := c.SearchGroupsForRoleWithResponse(ctx, ownerId, body)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	decodePage: decodeMembershipPage(func(r camunda.RoleGroupResult) string { return r.GroupId }),
}

func NewRoleMemberGroupResource() resource.Resource {
	return newMembershipResource(roleMemberGroup)
}

// searchAllRoleGroups reports whether memberId is assigned to ownerId, paging through all of the role's groups.
// Returns an error with message "not_found" if the role itself is not found.
func searchAllRoleGroups(ctx context.Context, client *camunda.ClientWithResponses, ownerId, memberId string) (bool, error) {
	return roleMemberGroup.contains(ctx, client, ownerId, memberId)
}
