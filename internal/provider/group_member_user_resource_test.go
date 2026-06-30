package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

func TestAccGroupMemberUserResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAccGroupMemberUserResourceConfig(),
				Check:  checkGroupUserAssignmentExistsInEngine("membertestgroup", "memberuser1"),
			},
		},
	})
}

func testAccGroupMemberUserResourceConfig() string {
	return `
resource "camundacluster_user" "memberuser" {
  username = "memberuser1"
  name     = "Member User"
  email    = "memberuser1@example.com"
  password = "testpass123"
}

resource "camundacluster_group" "membergroup" {
  name = "membertestgroup"
}

resource "camundacluster_group_member_user" "test" {
  group_id = camundacluster_group.membergroup.id
  user_id  = camundacluster_user.memberuser.id
}
`
}

func checkGroupUserAssignmentExistsInEngine(groupId, userId string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := camunda.NewClientWithResponses(testClusterURL)
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "camundacluster_group_member_user" {
				continue
			}
			gid := rs.Primary.Attributes["group_id"]
			uid := rs.Primary.Attributes["user_id"]
			searchResp, err := client.SearchUsersForGroupWithResponse(context.Background(), gid, camunda.SearchUsersForGroupJSONRequestBody{})
			if err != nil {
				return fmt.Errorf("engine API call failed: %w", err)
			}
			if searchResp.StatusCode() != 200 {
				return fmt.Errorf("search users for group %s failed (HTTP %d)", gid, searchResp.StatusCode())
			}
			var rawResult struct {
				Items []camunda.GroupUserResult `json:"items"`
			}
			if err := json.Unmarshal(searchResp.Body, &rawResult); err != nil {
				return fmt.Errorf("decode failed: %w", err)
			}
			for _, u := range rawResult.Items {
				if u.Username == uid {
					return nil
				}
			}
			return fmt.Errorf("user %s not found in group %s in engine", uid, gid)
		}
		return fmt.Errorf("group_member_user resource not found in Terraform state")
	}
}
