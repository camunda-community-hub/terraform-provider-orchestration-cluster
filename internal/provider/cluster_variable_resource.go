package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ClusterVariableResource{}
var _ resource.ResourceWithImportState = &ClusterVariableResource{}
var _ resource.ResourceWithModifyPlan = &ClusterVariableResource{}

func NewClusterVariableResource() resource.Resource {
	return &ClusterVariableResource{}
}

// ClusterVariableResource defines the resource implementation.
type ClusterVariableResource struct {
	client *camunda.ClientWithResponses
}

// ClusterVariableResourceModel describes the resource data model.
type ClusterVariableResourceModel struct {
	Id       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	TenantId types.String `tfsdk:"tenant_id"`
	Value    types.String `tfsdk:"value"`
	Scope    types.String `tfsdk:"scope"`
}

const (
	clusterVariableScopeGlobal = string(camunda.ClusterVariableScopeEnumGLOBAL)
	clusterVariableScopeTenant = string(camunda.ClusterVariableScopeEnumTENANT)
)

func (r *ClusterVariableResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_variable"
}

func (r *ClusterVariableResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Camunda cluster variable, either global or scoped to a single tenant. " +
			"The value is stored in the cluster's state and is **not** marked sensitive: do not store secrets in it.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The identity of the variable, which is also its import ID: `GLOBAL/<name>` for a global " +
					"variable or `TENANT/<tenant_id>/<name>` for a tenant-scoped one.",
				Computed: true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the cluster variable. Must be unique within its scope. Changing it replaces the variable.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the tenant the variable belongs to. Omit it to create a global variable. " +
					"Changing it (including adding or removing it) replaces the variable.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					nonEmptyStringValidator{},
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "The value of the variable as a JSON document: any JSON object, array, string, number, " +
					"boolean or `null`. Use `jsonencode()` to build it, e.g. `jsonencode(\"hello\")` for the string `hello` or " +
					"`jsonencode({ retries = 3 })` for an object. The value is stored exactly as configured; a value the API " +
					"reports back in a different but JSON-equivalent form (whitespace, key order) does not cause a diff.",
				Required: true,
				Validators: []validator.String{
					jsonValueValidator{},
				},
			},
			"scope": schema.StringAttribute{
				MarkdownDescription: "The scope of the variable: `GLOBAL` or `TENANT`.",
				Computed:            true,
			},
		},
	}
}

// jsonValueValidator validates that a string is a well-formed JSON document.
type jsonValueValidator struct{}

func (v jsonValueValidator) Description(ctx context.Context) string {
	return "must be a valid JSON document"
}

func (v jsonValueValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v jsonValueValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if !json.Valid([]byte(req.ConfigValue.ValueString())) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid JSON Value",
			fmt.Sprintf("%s must be a valid JSON document (use jsonencode() to build one).", req.Path),
		)
	}
}

// ModifyPlan derives the computed scope and id from the configured name and tenant_id, so they
// are known at plan time and stay correct when a changed tenant_id replaces the resource.
func (r *ClusterVariableResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var data ClusterVariableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Name.IsUnknown() || data.TenantId.IsUnknown() {
		return
	}

	var tenantId *string
	if !data.TenantId.IsNull() {
		tenantId = data.TenantId.ValueStringPointer()
	}

	data.Scope = types.StringValue(clusterVariableScope(tenantId))
	data.Id = types.StringValue(clusterVariableId(tenantId, data.Name.ValueString()))
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &data)...)
}

func (r *ClusterVariableResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*camunda.ClientWithResponses)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *camunda.ClientWithResponses, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *ClusterVariableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterVariableResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tenantId := tenantIdPointer(data.TenantId)
	name := data.Name.ValueString()

	apiResp, err := createClusterVariable(ctx, r.client, tenantId, name, data.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create cluster variable, got error: %s", err))
		return
	}

	if apiResp.Status != http.StatusOK {
		resp.Diagnostics.AddError("Not Created", fmt.Sprintf("Error while creating cluster variable, got HTTP error: %d: %s", apiResp.Status, apiResp.Body))
		return
	}

	if apiResp.Result == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	data.Name = types.StringValue(apiResp.Result.Name)
	data.TenantId = optionalStringValue(apiResp.Result.TenantId)
	data.Scope = types.StringValue(string(apiResp.Result.Scope))
	data.Id = types.StringValue(clusterVariableId(apiResp.Result.TenantId, apiResp.Result.Name))
	// data.Value keeps the configured value: the API echoes it back in its own serialization.

	// Persist state from the create response before polling for read consistency, so a
	// polling timeout or transport error doesn't orphan the variable the API already created.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := readClusterVariableWithRetry(ctx, r.client, tenantId, name); err != nil {
		resp.Diagnostics.AddWarning("Consistency Check Failed", fmt.Sprintf("Cluster variable %q was created but could not be confirmed readable yet: %s. State was saved from the create response; a later refresh will pick up any drift.", name, err))
		return
	}

	tflog.Trace(ctx, "created cluster variable resource")
}

func (r *ClusterVariableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterVariableResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tenantId := tenantIdPointer(data.TenantId)
	name := data.Name.ValueString()

	apiResp, err := getClusterVariable(ctx, r.client, tenantId, name)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read cluster variable '%s', got error: %s", name, err))
		return
	}

	if apiResp.Status == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if apiResp.Status != http.StatusOK {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read cluster variable '%s', got HTTP error: %d: %s", name, apiResp.Status, apiResp.Body))
		return
	}

	if apiResp.Result == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 200 but with no parseable JSON body")
		return
	}

	// The generated client drops the value field, so it is decoded from the raw body.
	value, err := parseClusterVariableValue(apiResp.Body)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Response", fmt.Sprintf("Unable to read the value of cluster variable '%s': %s", name, err))
		return
	}

	data.Name = types.StringValue(apiResp.Result.Name)
	data.TenantId = optionalStringValue(apiResp.Result.TenantId)
	data.Scope = types.StringValue(string(apiResp.Result.Scope))
	data.Id = types.StringValue(clusterVariableId(apiResp.Result.TenantId, apiResp.Result.Name))
	if data.Value.IsNull() || !jsonEquivalent(data.Value.ValueString(), value) {
		data.Value = types.StringValue(value)
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClusterVariableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ClusterVariableResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tenantId := tenantIdPointer(data.TenantId)
	name := data.Name.ValueString()

	apiResp, err := updateClusterVariable(ctx, r.client, tenantId, name, data.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update cluster variable, got error: %s", err))
		return
	}

	if apiResp.Status != http.StatusOK {
		resp.Diagnostics.AddError("Not Updated", fmt.Sprintf("Error while updating cluster variable, got HTTP error: %d: %s", apiResp.Status, apiResp.Body))
		return
	}

	if _, err := readClusterVariableUntilConsistent(ctx, r.client, tenantId, name, data.Value.ValueString()); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to confirm cluster variable update, got error: %s", err))
		return
	}

	// Save updated data into Terraform state; the configured value is kept as is.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClusterVariableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClusterVariableResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	status, err := deleteClusterVariable(ctx, r.client, tenantIdPointer(data.TenantId), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete cluster variable, got error: %s", err))
		return
	}

	if status != http.StatusNoContent && status != http.StatusNotFound {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting cluster variable, got HTTP error: %d", status))
		return
	}
}

func (r *ClusterVariableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tenantId, name, err := parseClusterVariableId(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tenant_id"), optionalStringValue(tenantId))...)
}

// clusterVariableScope returns the API scope for a variable with the given tenant ID: a nil
// tenant ID means a global variable.
func clusterVariableScope(tenantId *string) string {
	if tenantId == nil {
		return clusterVariableScopeGlobal
	}
	return clusterVariableScopeTenant
}

// clusterVariableId builds the resource ID and import ID of a variable: `GLOBAL/<name>` or
// `TENANT/<tenant_id>/<name>`. Tenant IDs never contain `/`, so the name may.
func clusterVariableId(tenantId *string, name string) string {
	if tenantId == nil || *tenantId == "" {
		return clusterVariableScopeGlobal + "/" + name
	}
	return clusterVariableScopeTenant + "/" + *tenantId + "/" + name
}

// parseClusterVariableId is the inverse of clusterVariableId. It returns a nil tenant ID for a
// global variable.
func parseClusterVariableId(id string) (tenantId *string, name string, err error) {
	invalid := fmt.Errorf("expected import ID in the format \"GLOBAL/<name>\" or \"TENANT/<tenant_id>/<name>\", got %q", id)

	scope, rest, found := strings.Cut(id, "/")
	if !found {
		return nil, "", invalid
	}

	switch scope {
	case clusterVariableScopeGlobal:
		if rest == "" {
			return nil, "", invalid
		}
		return nil, rest, nil
	case clusterVariableScopeTenant:
		tenant, name, found := strings.Cut(rest, "/")
		if !found || tenant == "" || name == "" {
			return nil, "", invalid
		}
		return &tenant, name, nil
	default:
		return nil, "", invalid
	}
}

// tenantIdPointer returns nil for a null or empty tenant_id (global scope).
func tenantIdPointer(v types.String) *string {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return nil
	}
	return v.ValueStringPointer()
}

// parseClusterVariableValue extracts the value from a get/create/update response body. The
// generated client drops it (the schema merges it in via allOf), and the API reports it as a
// string holding the serialized JSON value.
func parseClusterVariableValue(body []byte) (string, error) {
	var parsed struct {
		Value *string `json:"value"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if parsed.Value == nil {
		return "", fmt.Errorf("response has no value")
	}
	return *parsed.Value, nil
}

// jsonEquivalent reports whether a and b are JSON documents with the same content,
// ignoring whitespace and object key order. Numbers are compared at arbitrary precision
// (1 equals 1.0, but integers above 2^53 that differ stay different).
func jsonEquivalent(a, b string) bool {
	av, err := decodeJSONNumbers(a)
	if err != nil {
		return false
	}
	bv, err := decodeJSONNumbers(b)
	if err != nil {
		return false
	}
	return jsonValuesEqual(av, bv)
}

// decodeJSONNumbers decodes a JSON document, keeping numbers as json.Number.
func decodeJSONNumbers(s string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after JSON value")
	}
	return v, nil
}

func jsonValuesEqual(a, b any) bool {
	switch av := a.(type) {
	case json.Number:
		bv, ok := b.(json.Number)
		return ok && jsonNumbersEqual(av, bv)
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonValuesEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			other, ok := bv[k]
			if !ok || !jsonValuesEqual(v, other) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}

// jsonNumbersEqual compares two JSON numbers exactly. Numbers big.Rat cannot represent (such
// as an absurdly large exponent) fall back to comparing their text.
func jsonNumbersEqual(a, b json.Number) bool {
	ar, aok := new(big.Rat).SetString(a.String())
	br, bok := new(big.Rat).SetString(b.String())
	if aok && bok {
		return ar.Cmp(br) == 0
	}
	return a == b
}

// clusterVariableResponse is the status code, raw body and decoded result of a create, get or
// update call, which is the same for global and tenant-scoped variables.
type clusterVariableResponse struct {
	Status int
	Body   []byte
	Result *camunda.ClusterVariableResult
}

// clusterVariableRequestBody builds a create or update request body. The value is sent as raw
// JSON because the generated request types only model JSON objects, while a variable may hold
// any JSON value.
func clusterVariableRequestBody(name *string, value string) (*bytes.Reader, error) {
	body := struct {
		Name  *string         `json:"name,omitempty"`
		Value json.RawMessage `json:"value"`
	}{Name: name, Value: json.RawMessage(value)}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	return bytes.NewReader(encoded), nil
}

func createClusterVariable(ctx context.Context, client *camunda.ClientWithResponses, tenantId *string, name, value string) (clusterVariableResponse, error) {
	body, err := clusterVariableRequestBody(&name, value)
	if err != nil {
		return clusterVariableResponse{}, err
	}

	if tenantId == nil {
		resp, err := client.CreateGlobalClusterVariableWithBodyWithResponse(ctx, "application/json", body)
		if err != nil {
			return clusterVariableResponse{}, err
		}
		return clusterVariableResponse{Status: resp.StatusCode(), Body: resp.Body, Result: resp.JSON200}, nil
	}

	resp, err := client.CreateTenantClusterVariableWithBodyWithResponse(ctx, *tenantId, "application/json", body)
	if err != nil {
		return clusterVariableResponse{}, err
	}
	return clusterVariableResponse{Status: resp.StatusCode(), Body: resp.Body, Result: resp.JSON200}, nil
}

func getClusterVariable(ctx context.Context, client *camunda.ClientWithResponses, tenantId *string, name string) (clusterVariableResponse, error) {
	if tenantId == nil {
		resp, err := client.GetGlobalClusterVariableWithResponse(ctx, name)
		if err != nil {
			return clusterVariableResponse{}, err
		}
		return clusterVariableResponse{Status: resp.StatusCode(), Body: resp.Body, Result: resp.JSON200}, nil
	}

	resp, err := client.GetTenantClusterVariableWithResponse(ctx, *tenantId, name)
	if err != nil {
		return clusterVariableResponse{}, err
	}
	return clusterVariableResponse{Status: resp.StatusCode(), Body: resp.Body, Result: resp.JSON200}, nil
}

func updateClusterVariable(ctx context.Context, client *camunda.ClientWithResponses, tenantId *string, name, value string) (clusterVariableResponse, error) {
	body, err := clusterVariableRequestBody(nil, value)
	if err != nil {
		return clusterVariableResponse{}, err
	}

	if tenantId == nil {
		resp, err := client.UpdateGlobalClusterVariableWithBodyWithResponse(ctx, name, "application/json", body)
		if err != nil {
			return clusterVariableResponse{}, err
		}
		return clusterVariableResponse{Status: resp.StatusCode(), Body: resp.Body, Result: resp.JSON200}, nil
	}

	resp, err := client.UpdateTenantClusterVariableWithBodyWithResponse(ctx, *tenantId, name, "application/json", body)
	if err != nil {
		return clusterVariableResponse{}, err
	}
	return clusterVariableResponse{Status: resp.StatusCode(), Body: resp.Body, Result: resp.JSON200}, nil
}

func deleteClusterVariable(ctx context.Context, client *camunda.ClientWithResponses, tenantId *string, name string) (int, error) {
	if tenantId == nil {
		resp, err := client.DeleteGlobalClusterVariableWithResponse(ctx, name)
		if err != nil {
			return 0, err
		}
		return resp.StatusCode(), nil
	}

	resp, err := client.DeleteTenantClusterVariableWithResponse(ctx, *tenantId, name)
	if err != nil {
		return 0, err
	}
	return resp.StatusCode(), nil
}

// readClusterVariableWithRetry handles the eventual consistency of fetching a variable by
// retrying a few times: if a variable was just created, it may not be immediately available
// through the API.
func readClusterVariableWithRetry(ctx context.Context, client *camunda.ClientWithResponses, tenantId *string, name string) (clusterVariableResponse, error) {
	return readWithRetry(ctx, client, fmt.Sprintf("cluster variable %q", name),
		func() (clusterVariableResponse, error) {
			return getClusterVariable(ctx, client, tenantId, name)
		},
		func(readResp clusterVariableResponse) (bool, error) {
			switch readResp.Status {
			case http.StatusOK:
				return true, nil
			case http.StatusNotFound:
				return false, nil
			default:
				return false, fmt.Errorf("got HTTP error: %d: %s", readResp.Status, readResp.Body)
			}
		})
}

// readClusterVariableUntilConsistent polls GET until it reflects the given value, handling the
// same eventual consistency on updates as readClusterVariableWithRetry does on creates: the
// read-side projection can briefly return the pre-update value right after a successful PUT,
// which would otherwise make Terraform's post-apply refresh plan non-empty.
func readClusterVariableUntilConsistent(ctx context.Context, client *camunda.ClientWithResponses, tenantId *string, name, expectedValue string) (clusterVariableResponse, error) {
	return readUntilConsistent(ctx, client, fmt.Sprintf("cluster variable %q", name),
		func() (clusterVariableResponse, error) {
			return getClusterVariable(ctx, client, tenantId, name)
		},
		func(readResp clusterVariableResponse) (bool, error) {
			if readResp.Status == http.StatusNotFound {
				return false, nil
			}

			if readResp.Status != http.StatusOK {
				return false, fmt.Errorf("got HTTP error: %d: %s", readResp.Status, readResp.Body)
			}

			value, err := parseClusterVariableValue(readResp.Body)
			if err != nil {
				return false, err
			}

			return jsonEquivalent(value, expectedValue), nil
		})
}
