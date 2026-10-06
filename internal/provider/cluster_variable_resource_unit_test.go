package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	create, err := clusterVariableRequestBody(&name, `"text"`)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, create.Len())
	_, _ = create.Read(buf)
	if string(buf) != `{"name":"n","value":"text"}` {
		t.Fatalf("create body: %s", buf)
	}

	update, err := clusterVariableRequestBody(nil, `{"a":1}`)
	if err != nil {
		t.Fatal(err)
	}
	buf = make([]byte, update.Len())
	_, _ = update.Read(buf)
	if string(buf) != `{"value":{"a":1}}` {
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
