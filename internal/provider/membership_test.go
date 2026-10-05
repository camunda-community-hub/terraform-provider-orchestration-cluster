package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
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
	}{
		{
			name:         "found on first page stops immediately",
			pages:        []string{`{"items":[{"clientId":"a"},{"clientId":"target"}],"page":{"endCursor":"c1","hasMoreTotalItems":true}}`},
			want:         true,
			wantRequests: 1,
		},
		{
			name: "follows cursor to later page",
			pages: []string{
				`{"items":[{"clientId":"a"}],"page":{"endCursor":"c1"}}`,
				`{"items":[{"clientId":"target"}],"page":{"endCursor":"c2"}}`,
			},
			want:         true,
			wantRequests: 2,
		},
		{
			name: "stops when end cursor is empty",
			pages: []string{
				`{"items":[{"clientId":"a"}],"page":{"endCursor":"c1"}}`,
				`{"items":[{"clientId":"b"}],"page":{"endCursor":""}}`,
			},
			want:         false,
			wantRequests: 2,
		},
		{
			name:         "stops when end cursor is absent",
			pages:        []string{`{"items":[{"clientId":"a"}],"page":{}}`},
			want:         false,
			wantRequests: 1,
		},
		{
			name: "stops when a page has no items even with a cursor",
			pages: []string{
				`{"items":[{"clientId":"a"}],"page":{"endCursor":"c1"}}`,
				`{"items":[],"page":{"endCursor":"c2","hasMoreTotalItems":true}}`,
			},
			want:         false,
			wantRequests: 2,
		},
		{
			name: "stops when the exact total is consumed",
			pages: []string{
				`{"items":[{"clientId":"a"}],"page":{"endCursor":"c1","totalItems":2}}`,
				`{"items":[{"clientId":"b"}],"page":{"endCursor":"c2","totalItems":2}}`,
			},
			want:         false,
			wantRequests: 2,
		},
		{
			name: "ignores a capped total",
			pages: []string{
				`{"items":[{"clientId":"a"}],"page":{"endCursor":"c1","totalItems":1,"hasMoreTotalItems":true}}`,
				`{"items":[{"clientId":"target"}],"page":{"endCursor":"c2","totalItems":1,"hasMoreTotalItems":true}}`,
			},
			want:         true,
			wantRequests: 2,
		},
		{
			name: "stops when the cursor does not advance",
			pages: []string{
				`{"items":[{"clientId":"a"}],"page":{"endCursor":"c1"}}`,
				`{"items":[{"clientId":"b"}],"page":{"endCursor":"c1"}}`,
			},
			want:         false,
			wantRequests: 2,
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
			if tt.wantRequests > 1 {
				page, _ := reqs[1]["page"].(map[string]any)
				if page["after"] != "c1" {
					t.Errorf("second request page = %v, want after=c1", page)
				}
			}
		})
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
