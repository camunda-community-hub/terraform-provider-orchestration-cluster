package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.10"
)

// membershipSearchServer serves the given pages of a tenant client search in order, recording
// the request bodies it received.
func membershipSearchServer(t *testing.T, pages []string) (*camunda.ClientWithResponses, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests = append(requests, body)
		if len(requests) > len(pages) {
			t.Errorf("unexpected request %d, only %d pages served", len(requests), len(pages))
			http.Error(w, "too many requests", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pages[len(requests)-1]))
	}))
	t.Cleanup(server.Close)
	client, err := camunda.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return client, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return requests
	}
}

func TestMembershipContains_pagination(t *testing.T) {
	tests := []struct {
		name         string
		pages        []string
		want         bool
		wantRequests int
		// wantFroms is the page.from expected on each request after the first.
		wantFroms []float64
	}{
		{
			name:         "found on first page stops immediately",
			pages:        []string{`{"items":[{"clientId":"a"},{"clientId":"target"}],"page":{"hasMoreTotalItems":true}}`},
			want:         true,
			wantRequests: 1,
		},
		{
			name: "follows the offset to a later page",
			pages: []string{
				`{"items":[{"clientId":"a"}],"page":{}}`,
				`{"items":[{"clientId":"target"}],"page":{}}`,
			},
			want:         true,
			wantRequests: 2,
			wantFroms:    []float64{1},
		},
		{
			name: "advances the offset by the items seen",
			pages: []string{
				`{"items":[{"clientId":"a"},{"clientId":"b"}],"page":{}}`,
				`{"items":[{"clientId":"c"}],"page":{}}`,
				`{"items":[],"page":{}}`,
			},
			want:         false,
			wantRequests: 3,
			wantFroms:    []float64{2, 3},
		},
		{
			name:         "stops when the first page has no items",
			pages:        []string{`{"items":[],"page":{}}`},
			want:         false,
			wantRequests: 1,
		},
		{
			name: "ignores the end cursor",
			pages: []string{
				`{"items":[{"clientId":"a"}],"page":{"endCursor":"c1"}}`,
				`{"items":[{"clientId":"target"}],"page":{"endCursor":""}}`,
			},
			want:         true,
			wantRequests: 2,
			wantFroms:    []float64{1},
		},
		{
			name: "ignores an exact total and pages until a page is empty",
			pages: []string{
				`{"items":[{"clientId":"a"}],"page":{"totalItems":2}}`,
				`{"items":[{"clientId":"b"}],"page":{"totalItems":2}}`,
				`{"items":[],"page":{"totalItems":2}}`,
			},
			want:         false,
			wantRequests: 3,
			wantFroms:    []float64{1, 2},
		},
		{
			name: "ignores a capped total",
			pages: []string{
				`{"items":[{"clientId":"a"}],"page":{"totalItems":1,"hasMoreTotalItems":true}}`,
				`{"items":[{"clientId":"target"}],"page":{"totalItems":1,"hasMoreTotalItems":true}}`,
			},
			want:         true,
			wantRequests: 2,
			wantFroms:    []float64{1},
		},
		{
			name: "stops when a page repeats the previous one",
			pages: []string{
				`{"items":[{"clientId":"a"}],"page":{}}`,
				`{"items":[{"clientId":"a"}],"page":{}}`,
			},
			want:         false,
			wantRequests: 2,
			wantFroms:    []float64{1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, requests := membershipSearchServer(t, tt.pages)
			got, err := tenantMemberClient.contains(context.Background(), client, "tenant", "target")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("contains = %v, want %v", got, tt.want)
			}
			reqs := requests()
			if len(reqs) != tt.wantRequests {
				t.Fatalf("got %d requests, want %d", len(reqs), tt.wantRequests)
			}
			if _, hasPage := reqs[0]["page"]; hasPage {
				t.Errorf("first request must omit page, got %v", reqs[0])
			}
			for i, wantFrom := range tt.wantFroms {
				page, _ := reqs[i+1]["page"].(map[string]any)
				if page["from"] != wantFrom || page["limit"] != float64(membershipPageSize) {
					t.Errorf("request %d page = %v, want from=%v limit=%d", i+2, page, wantFrom, membershipPageSize)
				}
				if _, hasAfter := page["after"]; hasAfter {
					t.Errorf("request %d must not page by cursor, got %v", i+2, page)
				}
			}
		})
	}
}

// TestMembershipContains_groupLargerThanOnePage serves a group of 160 users the way a SaaS
// cluster was observed to: the first request without a page returns 100 items, but the end
// cursor of that page is Long.MAX_VALUE, so searching after it returns nothing (see
// camunda/camunda#68013). Requests with an offset are honoured. Every member, including those
// past the first page, must still be found.
func TestMembershipContains_groupLargerThanOnePage(t *testing.T) {
	const total = 160
	const defaultPageSize = 100
	const brokenEndCursor = "WzkyMjMzNzIwMzY4NTQ3NzU4MDdd" // base64 of [9223372036854775807]
	usernames := make([]string, total)
	for i := range usernames {
		usernames[i] = fmt.Sprintf("user%03d@example.com", i)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Page struct {
				After string `json:"after"`
				From  int    `json:"from"`
				Limit int    `json:"limit"`
			} `json:"page"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		start, limit := body.Page.From, defaultPageSize
		if body.Page.Limit > 0 {
			limit = body.Page.Limit
		}
		end := min(start+limit, total)
		if body.Page.After != "" || start > total {
			start, end = 0, 0
		}

		items := make([]map[string]string, 0, end-start)
		for _, u := range usernames[start:end] {
			items = append(items, map[string]string{"username": u})
		}
		page := map[string]any{"totalItems": total, "hasMoreTotalItems": false}
		if len(items) > 0 {
			page["endCursor"] = brokenEndCursor
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "page": page})
	}))
	t.Cleanup(server.Close)
	client, err := camunda.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	for i, username := range usernames {
		got, err := groupMemberUser.contains(context.Background(), client, "PDPTaskUser", username)
		if err != nil {
			t.Fatalf("%s: %v", username, err)
		}
		if !got {
			t.Errorf("member %d (%s) not found", i, username)
		}
	}

	got, err := groupMemberUser.contains(context.Background(), client, "PDPTaskUser", "absent@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("absent member reported as found")
	}

	all, err := memberSearchOf(groupMemberUser).all(context.Background(), client, "PDPTaskUser")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != total {
		t.Errorf("all returned %d members, want %d", len(all), total)
	}
}

func TestMembershipContains_errors(t *testing.T) {
	for _, tt := range []struct {
		status  int
		wantErr string
	}{
		{http.StatusNotFound, "not_found"},
		{http.StatusInternalServerError, "HTTP 500"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tt.status)
		}))
		client, err := camunda.NewClientWithResponses(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tenantMemberClient.contains(context.Background(), client, "tenant", "target")
		server.Close()
		if err == nil || err.Error() != tt.wantErr {
			t.Errorf("status %d: err = %v, want %q", tt.status, err, tt.wantErr)
		}
	}
}

func TestMembershipId_roundTrip(t *testing.T) {
	owner, member := "a/b", "c%d/e"
	id := encodeMembershipId(owner, member)
	gotOwner, gotMember, err := decodeMembershipId(id)
	if err != nil {
		t.Fatal(err)
	}
	if gotOwner != owner || gotMember != member {
		t.Errorf("round trip = %q, %q; want %q, %q", gotOwner, gotMember, owner, member)
	}
	for _, bad := range []string{"", "a", "/b", "a/", "%zz/b"} {
		if _, _, err := decodeMembershipId(bad); err == nil {
			t.Errorf("decodeMembershipId(%q) succeeded, want error", bad)
		}
	}
}
