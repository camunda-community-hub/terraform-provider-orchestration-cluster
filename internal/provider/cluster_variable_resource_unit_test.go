package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.10"
)

func TestClusterVariableId(t *testing.T) {
	tenant := "acme"

	tests := []struct {
		name     string
		tenantId *string
		varName  string
		want     string
	}{
		{"global", nil, "limit", "GLOBAL/limit"},
		{"empty tenant is global", new(string), "limit", "GLOBAL/limit"},
		{"tenant", &tenant, "limit", "TENANT/acme/limit"},
		{"name with slash", &tenant, "a/b", "TENANT/acme/a/b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clusterVariableId(tt.tenantId, tt.varName)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}

			gotTenant, gotName, err := parseClusterVariableId(got)
			if err != nil {
				t.Fatalf("parse: %s", err)
			}
			if gotName != tt.varName {
				t.Fatalf("name: got %q, want %q", gotName, tt.varName)
			}
			if (tt.tenantId == nil || *tt.tenantId == "") != (gotTenant == nil) || (gotTenant != nil && *gotTenant != *tt.tenantId) {
				t.Fatalf("tenant: got %v, want %v", gotTenant, tt.tenantId)
			}
		})
	}
}

func TestParseClusterVariableId_invalid(t *testing.T) {
	for _, id := range []string{"", "limit", "GLOBAL/", "global/limit", "TENANT/acme", "TENANT//limit", "TENANT/acme/", "OTHER/acme/limit"} {
		if _, _, err := parseClusterVariableId(id); err == nil {
			t.Errorf("expected error for %q", id)
		}
	}
}

func TestParseClusterVariableValue(t *testing.T) {
	got, err := parseClusterVariableValue([]byte(`{"name":"n","scope":"GLOBAL","tenantId":null,"value":"{\"a\":1}"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"a":1}` {
		t.Fatalf("got %q", got)
	}

	for _, body := range []string{``, `not json`, `{"name":"n"}`, `{"value":{"a":1}}`} {
		if _, err := parseClusterVariableValue([]byte(body)); err == nil {
			t.Errorf("expected error for %q", body)
		}
	}
}

func TestJsonEquivalent(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{`{"a":1,"b":[1,2]}`, `{ "b": [1, 2], "a": 1 }`, true},
		{`"x"`, ` "x" `, true},
		{`1`, `1.0`, true},
		{`{"a":1}`, `{"a":2}`, false},
		{`"1"`, `1`, false},
		{`[1,2]`, `[2,1]`, false},
		{`not json`, `not json`, false},
		{`{"a":1} {"a":1}`, `{"a":1}`, false},
		{`9007199254740993`, `9007199254740992`, false},
		{`9007199254740993`, `9007199254740993.0`, true},
		{`1e1000`, `1e1000`, true},
		{`{"a":[1e2]}`, `{"a":[100]}`, true},
	}
	for _, tt := range tests {
		if got := jsonEquivalent(tt.a, tt.b); got != tt.want {
			t.Errorf("jsonEquivalent(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestClusterVariableRequestBody(t *testing.T) {
	name := "n"
	kind := "SECRET_REFERENCE"
	create, err := clusterVariableRequestBody(&name, &kind, `"text"`, map[string]string{"team": "a"})
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, create.Len())
	_, _ = create.Read(buf)
	if string(buf) != `{"name":"n","kind":"SECRET_REFERENCE","value":"text","metadata":{"team":"a"}}` {
		t.Fatalf("create body: %s", buf)
	}

	update, err := clusterVariableRequestBody(nil, nil, `{"a":1}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf = make([]byte, update.Len())
	_, _ = update.Read(buf)
	if string(buf) != `{"value":{"a":1},"metadata":{}}` {
		t.Fatalf("update body: %s", buf)
	}
}

func TestJsonValueValidator(t *testing.T) {
	tests := []struct {
		value   types.String
		wantErr bool
	}{
		{types.StringValue(`{"a":1}`), false},
		{types.StringValue(`"x"`), false},
		{types.StringValue(`null`), false},
		{types.StringValue(`not json`), true},
		{types.StringValue(``), true},
		{types.StringNull(), false},
		{types.StringUnknown(), false},
	}
	for _, tt := range tests {
		resp := &validator.StringResponse{}
		jsonValueValidator{}.ValidateString(context.Background(), validator.StringRequest{ConfigValue: tt.value}, resp)
		if resp.Diagnostics.HasError() != tt.wantErr {
			t.Errorf("value %s: error = %v, want %v", tt.value, resp.Diagnostics.HasError(), tt.wantErr)
		}
	}
}

func TestParseClusterVariableMetadata(t *testing.T) {
	got, err := parseClusterVariableMetadata([]byte(`{"metadata":{"team":"a","retries":3,"ratio":0.5}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"team": "a", "retries": "3", "ratio": "0.5"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("metadata[%q] = %q, want %q", k, got[k], v)
		}
	}

	empty, err := parseClusterVariableMetadata([]byte(`{"name":"n"}`))
	if err != nil || len(empty) != 0 {
		t.Errorf("a response without metadata must yield an empty map, got %v, %v", empty, err)
	}
}

func TestReconcileClusterVariableMetadata(t *testing.T) {
	ctx := context.Background()
	empty := types.MapValueMust(types.StringType, map[string]attr.Value{})
	filled := types.MapValueMust(types.StringType, map[string]attr.Value{"team": types.StringValue("a")})

	tests := []struct {
		name     string
		prior    types.Map
		api      map[string]string
		wantNull bool
		wantLen  int
	}{
		{"configured empty map is kept when the API has none", empty, map[string]string{}, false, 0},
		{"omitted metadata stays null when the API has none", types.MapNull(types.StringType), map[string]string{}, true, 0},
		{"unknown prior becomes null when the API has none", types.MapUnknown(types.StringType), map[string]string{}, true, 0},
		{"API metadata wins over a configured empty map", empty, map[string]string{"team": "a"}, false, 1},
		{"API metadata wins over a configured value", filled, map[string]string{"env": "x", "team": "b"}, false, 2},
		{"cleared out of band becomes null", filled, map[string]string{}, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, diags := reconcileClusterVariableMetadata(ctx, tt.prior, tt.api)
			if diags.HasError() {
				t.Fatal(diags)
			}
			if got.IsNull() != tt.wantNull || len(got.Elements()) != tt.wantLen {
				t.Errorf("got null=%v len=%d, want null=%v len=%d", got.IsNull(), len(got.Elements()), tt.wantNull, tt.wantLen)
			}
		})
	}
}

// TestReadClusterVariableUntilConsistent_waitsForRecreatedVariable covers a variable that is
// replaced under the same name (for example when its kind changes): the eventually consistent GET
// first returns the old variable, which must not be accepted as the result of the create.
func TestReadClusterVariableUntilConsistent_waitsForRecreatedVariable(t *testing.T) {
	withFastConsistencyPolling(t)

	responses := []string{
		`{"name":"v","scope":"GLOBAL","tenantId":null,"kind":"JSON","metadata":{},"value":"\"old\""}`,
		`{"name":"v","scope":"GLOBAL","tenantId":null,"kind":"SECRET_REFERENCE","metadata":{},"value":"\"new\""}`,
		`{"name":"v","scope":"GLOBAL","tenantId":null,"kind":"SECRET_REFERENCE","metadata":{"team":"a"},"value":"\"new\""}`,
	}
	var mu sync.Mutex
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		i := min(calls, len(responses)-1)
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responses[i]))
	}))
	t.Cleanup(server.Close)
	client, err := camunda.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	got, err := readClusterVariableUntilConsistent(context.Background(), client, nil, "v", "SECRET_REFERENCE", `"new"`, map[string]string{"team": "a"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Result.Kind) != "SECRET_REFERENCE" {
		t.Errorf("kind = %q, want SECRET_REFERENCE", got.Result.Kind)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 3 {
		t.Errorf("got %d reads, want 3: the old kind, the old metadata and then the recreated variable", calls)
	}
}
