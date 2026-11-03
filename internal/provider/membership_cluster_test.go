package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"testing"
	"time"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.10"
)

// TestAccMembership_groupLargerThanOnePage assigns more users to one group than fit on a
// search page and checks that every assignment becomes visible through the same lookup the
// provider uses to confirm an assignment. It runs against a real cluster, so it shows how the
// cluster actually pages a group's members, which the fake servers in membership_test.go
// can only assume.
func TestAccMembership_groupLargerThanOnePage(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run against a real cluster")
	}
	testAccPreCheck(t)

	const members = 150
	ctx := t.Context()
	client, err := camunda.NewClientWithResponses(testClusterURL)
	if err != nil {
		t.Fatal(err)
	}

	groupId := fmt.Sprintf("pagegroup%d", time.Now().UnixNano())
	createGroup, err := client.CreateGroupWithResponse(ctx, camunda.CreateGroupJSONRequestBody{GroupId: groupId, Name: groupId})
	if err != nil {
		t.Fatal(err)
	}
	if createGroup.StatusCode() != http.StatusCreated {
		t.Fatalf("create group: HTTP %d: %s", createGroup.StatusCode(), createGroup.Body)
	}
	t.Cleanup(func() { _, _ = client.DeleteGroupWithResponse(context.WithoutCancel(ctx), groupId) })

	usernames := make([]string, members)
	for i := range usernames {
		usernames[i] = fmt.Sprintf("%s-user%03d", groupId, i)
		createUser, err := client.CreateUserWithResponse(ctx, camunda.CreateUserJSONRequestBody{Username: usernames[i], Password: "Passw0rd!"})
		if err != nil {
			t.Fatal(err)
		}
		if createUser.StatusCode() != http.StatusCreated {
			t.Fatalf("create user %s: HTTP %d: %s", usernames[i], createUser.StatusCode(), createUser.Body)
		}
		username := usernames[i]
		t.Cleanup(func() { _, _ = client.DeleteUserWithResponse(context.WithoutCancel(ctx), username) })

		assign, err := client.AssignUserToGroupWithResponse(ctx, groupId, username)
		if err != nil {
			t.Fatal(err)
		}
		if assign.StatusCode() != http.StatusNoContent {
			t.Fatalf("assign %s: HTTP %d: %s", username, assign.StatusCode(), assign.Body)
		}
	}

	// Log how the cluster pages the group, so a failure below can be traced to what the
	// cluster returned rather than to the provider.
	logGroupPages(t, ctx, client, groupId)

	var missing []string
	for _, username := range usernames {
		found, err := waitForConsistency(ctx, nil, fmt.Sprintf("user %q in group %q", username, groupId), func() (bool, bool, error) {
			found, err := groupMemberUser.contains(ctx, client, groupId, username)
			if err != nil {
				return false, false, err
			}
			return found, found, nil
		})
		if err != nil || !found {
			missing = append(missing, username)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d of %d assigned users never became visible through contains, first missing: %v", len(missing), members, missing[:min(len(missing), 5)])
	}

	all, err := memberSearch{search: groupMemberUser.search, decodePage: groupMemberUser.decodePage}.all(ctx, client, groupId)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != members {
		t.Errorf("all returned %d members, want %d", len(all), members)
	}
	for _, username := range usernames {
		if !slices.Contains(all, username) {
			t.Errorf("all is missing %s", username)
		}
	}
}

// logGroupPages logs the shape of the first two pages of the group's user search: the item
// count, end cursor and totals. The second page is requested by offset, as forEachPage does.
func logGroupPages(t *testing.T, ctx context.Context, client *camunda.ClientWithResponses, groupId string) {
	t.Helper()

	var seen int32
	for page := 1; page <= 2; page++ {
		body := camunda.SearchQueryRequest{}
		if seen > 0 {
			limit := membershipPageSize
			from := seen
			sqpr := camunda.SearchQueryPageRequest{}
			if err := sqpr.FromOffsetPagination(camunda.OffsetPagination{From: &from, Limit: &limit}); err != nil {
				t.Fatal(err)
			}
			body.Page = &sqpr
		}
		resp, err := client.SearchUsersForGroupWithResponse(ctx, groupId, body)
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Items []json.RawMessage `json:"items"`
			Page  struct {
				EndCursor         *string `json:"endCursor"`
				TotalItems        *int64  `json:"totalItems"`
				HasMoreTotalItems bool    `json:"hasMoreTotalItems"`
			} `json:"page"`
		}
		if err := json.Unmarshal(resp.Body, &decoded); err != nil {
			t.Fatalf("page %d: HTTP %d: %s", page, resp.StatusCode(), resp.Body)
		}
		endCursor := ""
		if decoded.Page.EndCursor != nil {
			endCursor = *decoded.Page.EndCursor
		}
		totalItems := int64(-1)
		if decoded.Page.TotalItems != nil {
			totalItems = *decoded.Page.TotalItems
		}
		t.Logf("page %d (from %d): HTTP %d, %d items, endCursor=%q, totalItems=%d, hasMoreTotalItems=%v",
			page, seen, resp.StatusCode(), len(decoded.Items), endCursor, totalItems, decoded.Page.HasMoreTotalItems)
		if len(decoded.Items) == 0 {
			return
		}
		seen += int32(len(decoded.Items))
	}
}
