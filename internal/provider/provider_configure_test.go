package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

// objectOf builds an object value of type typ, setting unspecified attributes to null.
func objectOf(typ tftypes.Type, vals map[string]tftypes.Value) tftypes.Value {
	attrs := map[string]tftypes.Value{}
	obj, ok := typ.(tftypes.Object)
	if !ok {
		panic(fmt.Sprintf("objectOf: %T is not a tftypes.Object", typ))
	}
	for name, attrType := range obj.AttributeTypes {
		if v, ok := vals[name]; ok {
			attrs[name] = v
		} else {
			attrs[name] = tftypes.NewValue(attrType, nil)
		}
	}
	return tftypes.NewValue(typ, attrs)
}

// configureProvider runs the provider's Configure with the given attribute
// values. Nested blocks are given as already-built values.
func configureProvider(t *testing.T, vals map[string]tftypes.Value) *provider.ConfigureResponse {
	t.Helper()
	ctx := context.Background()
	p := New("test")()

	var schemaResp provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)
	typ := schemaResp.Schema.Type().TerraformType(ctx)

	resp := &provider.ConfigureResponse{}
	p.Configure(ctx, provider.ConfigureRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: objectOf(typ, vals)},
	}, resp)
	return resp
}

func nestedObject(t *testing.T, name string, vals map[string]string) tftypes.Value {
	t.Helper()
	ctx := context.Background()
	var schemaResp provider.SchemaResponse
	New("test")().Schema(ctx, provider.SchemaRequest{}, &schemaResp)
	typ := schemaResp.Schema.Attributes[name].GetType().TerraformType(ctx)
	converted := map[string]tftypes.Value{}
	for k, v := range vals {
		converted[k] = tftypes.NewValue(tftypes.String, v)
	}
	return objectOf(typ, converted)
}

// authorizationSeenByServer configures the provider against a fake API, performs
// one request with the resulting client and returns the Authorization header the
// server received.
func authorizationSeenByServer(t *testing.T, build func(url string) map[string]tftypes.Value) string {
	t.Helper()
	var got string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer api.Close()

	resp := configureProvider(t, build(api.URL))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	client, ok := resp.ResourceData.(*camunda.ClientWithResponses)
	if !ok {
		t.Fatalf("ResourceData is %T, want *ClientWithResponses", resp.ResourceData)
	}
	if resp.DataSourceData != resp.ResourceData {
		t.Error("DataSourceData must share the client with ResourceData")
	}
	if _, err := client.GetTopologyWithResponse(context.Background()); err != nil {
		t.Fatalf("request failed: %s", err)
	}
	return got
}

func TestProviderConfigure_NoAuth(t *testing.T) {
	got := authorizationSeenByServer(t, func(url string) map[string]tftypes.Value {
		return map[string]tftypes.Value{"url": tftypes.NewValue(tftypes.String, url)}
	})
	if got != "" {
		t.Errorf("Authorization = %q, want none", got)
	}
}

func TestProviderConfigure_BasicAuth(t *testing.T) {
	got := authorizationSeenByServer(t, func(url string) map[string]tftypes.Value {
		return map[string]tftypes.Value{
			"url":        tftypes.NewValue(tftypes.String, url),
			"basic_auth": nestedObject(t, "basic_auth", map[string]string{"username": "demo", "password": "s3cret"}),
		}
	})
	// base64("demo:s3cret")
	if want := "Basic ZGVtbzpzM2NyZXQ="; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestProviderConfigure_OIDC(t *testing.T) {
	var form map[string][]string
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		_, _ = w.Write([]byte(`{"access_token":"tok-123"}`))
	}))
	defer idp.Close()

	got := authorizationSeenByServer(t, func(url string) map[string]tftypes.Value {
		return map[string]tftypes.Value{
			"url": tftypes.NewValue(tftypes.String, url),
			"oidc": nestedObject(t, "oidc", map[string]string{
				"client_id":     "my-client",
				"client_secret": "my-secret",
				"login_url":     idp.URL,
				"audience":      "my-audience",
			}),
		}
	})
	if want := "Bearer tok-123"; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
	for k, want := range map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     "my-client",
		"client_secret": "my-secret",
		"audience":      "my-audience",
	} {
		if len(form[k]) != 1 || form[k][0] != want {
			t.Errorf("token request %s = %v, want %q", k, form[k], want)
		}
	}
}

func TestProviderConfigure_OIDCLoginFailure(t *testing.T) {
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer idp.Close()

	resp := configureProvider(t, map[string]tftypes.Value{
		"url": tftypes.NewValue(tftypes.String, "http://localhost:1"),
		"oidc": nestedObject(t, "oidc", map[string]string{
			"client_id": "c", "client_secret": "s", "login_url": idp.URL, "audience": "a",
		}),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); !strings.Contains(got, "Authentication Failed") {
		t.Errorf("summary = %q, want Authentication Failed", got)
	}
	if resp.ResourceData != nil {
		t.Error("no client must be provided when authentication fails")
	}
}
