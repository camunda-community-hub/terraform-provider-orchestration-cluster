package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &MappingRuleResource{}
var _ resource.ResourceWithImportState = &MappingRuleResource{}

func NewMappingRuleResource() resource.Resource {
	return &MappingRuleResource{}
}

// MappingRuleResource defines the resource implementation.
type MappingRuleResource struct {
	client *camunda.ClientWithResponses
}

// MappingRuleResourceModel describes the resource data model.
type MappingRuleResourceModel struct {
	ClaimName     types.String `tfsdk:"claim_name"`
	ClaimValue    types.String `tfsdk:"claim_value"`
	MappingRuleId types.String `tfsdk:"mapping_rule_id"`
	Name          types.String `tfsdk:"name"`
}

func (r *MappingRuleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mapping_rule"
}

func (r *MappingRuleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Camunda cluster mapping rule",

		Attributes: map[string]schema.Attribute{
			"claim_name": schema.StringAttribute{
				MarkdownDescription: "The name of the claim to map.",
				Required:            true,
			},
			"claim_value": schema.StringAttribute{
				MarkdownDescription: "The value of the claim to map.",
				Required:            true,
			},
			"mapping_rule_id": schema.StringAttribute{
				MarkdownDescription: "The unique ID for the mapping rule.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the mapping rule.",
				Required:            true,
			},
		},
	}
}

func (r *MappingRuleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MappingRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data MappingRuleResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// The generated CreateMappingRuleJSONRequestBody type is missing the required
	// mappingRuleId field (an oapi-codegen limitation with the spec's allOf composition), so
	// the request body is built and sent manually here instead of via the typed client method.
	body, err := json.Marshal(struct {
		MappingRuleId string `json:"mappingRuleId"`
		ClaimName     string `json:"claimName"`
		ClaimValue    string `json:"claimValue"`
		Name          string `json:"name"`
	}{
		MappingRuleId: data.MappingRuleId.ValueString(),
		ClaimName:     data.ClaimName.ValueString(),
		ClaimValue:    data.ClaimValue.ValueString(),
		Name:          data.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to marshal mapping rule request, got error: %s", err))
		return
	}

	apiResp, err := r.client.CreateMappingRuleWithBodyWithResponse(ctx, "application/json", bytes.NewReader(body))

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create mapping rule, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusCreated {
		resp.Diagnostics.AddError("Not Created", fmt.Sprintf("Error while creating mapping rule, got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body))
		return
	}

	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError("Invalid Response", "Server returned 201 but with no parseable JSON body")
		return
	}

	data.MappingRuleId = types.StringValue(apiResp.JSON201.MappingRuleId)
	data.ClaimName = types.StringValue(apiResp.JSON201.ClaimName)
	data.ClaimValue = types.StringValue(apiResp.JSON201.ClaimValue)
	data.Name = types.StringValue(apiResp.JSON201.Name)

	// Persist state from the create response before polling for read consistency, so a
	// polling timeout or transport error doesn't orphan the mapping rule the API already created.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := readMappingRuleWithRetry(ctx, r.client, apiResp.JSON201.MappingRuleId); err != nil {
		resp.Diagnostics.AddWarning("Consistency Check Failed", fmt.Sprintf("Mapping rule %q was created but could not be confirmed readable yet: %s. State was saved from the create response; a later refresh will pick up any drift.", apiResp.JSON201.MappingRuleId, err))
		return
	}

	tflog.Trace(ctx, "created mapping rule resource")
}

func (r *MappingRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data MappingRuleResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := readMappingRuleWithRetry(ctx, r.client, data.MappingRuleId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read mapping rule '%s', got error: %s", data.MappingRuleId.ValueString(), err))
		return
	}

	data.MappingRuleId = types.StringValue(apiResp.JSON200.MappingRuleId)
	data.ClaimName = types.StringValue(apiResp.JSON200.ClaimName)
	data.ClaimValue = types.StringValue(apiResp.JSON200.ClaimValue)
	data.Name = types.StringValue(apiResp.JSON200.Name)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MappingRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data MappingRuleResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	request := camunda.UpdateMappingRuleJSONRequestBody{
		ClaimName:  data.ClaimName.ValueString(),
		ClaimValue: data.ClaimValue.ValueString(),
		Name:       data.Name.ValueString(),
	}
	apiResp, err := r.client.UpdateMappingRuleWithResponse(ctx, data.MappingRuleId.ValueString(), request)

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update mapping rule, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError("Not Updated", fmt.Sprintf("Error while updating mapping rule, got HTTP error: %d", apiResp.StatusCode()))
		return
	}

	readResp, err := readMappingRuleUntilConsistent(ctx, r.client, data.MappingRuleId.ValueString(), data.ClaimName.ValueString(), data.ClaimValue.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to confirm mapping rule update, got error: %s", err))
		return
	}

	data.MappingRuleId = types.StringValue(readResp.JSON200.MappingRuleId)
	data.ClaimName = types.StringValue(readResp.JSON200.ClaimName)
	data.ClaimValue = types.StringValue(readResp.JSON200.ClaimValue)
	data.Name = types.StringValue(readResp.JSON200.Name)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MappingRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data MappingRuleResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.DeleteMappingRuleWithResponse(ctx, data.MappingRuleId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete mapping rule, got error: %s", err))
		return
	}

	if apiResp.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Not Deleted", fmt.Sprintf("Error while deleting mapping rule, got HTTP error: %d", apiResp.StatusCode()))
		return
	}
}

func (r *MappingRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("mapping_rule_id"), req, resp)
}

// readMappingRuleWithRetry handles the eventual consistency of fetching a mapping rule by retrying a few times:
// if a mapping rule was just created, it may not be immediately available through the API.
func readMappingRuleWithRetry(ctx context.Context, client *camunda.ClientWithResponses, mappingRuleId string) (*camunda.GetMappingRuleResponse, error) {
	return waitForConsistency(ctx, fmt.Sprintf("mapping rule %q", mappingRuleId), func() (*camunda.GetMappingRuleResponse, bool, error) {
		readResp, err := client.GetMappingRuleWithResponse(ctx, mappingRuleId)
		if err != nil {
			return nil, false, err
		}

		return readResp, readResp.StatusCode() == http.StatusOK, nil
	})
}

// readMappingRuleUntilConsistent polls GET until it reflects the given claim name, claim value,
// and name, handling the same eventual consistency on updates as readMappingRuleWithRetry does on
// creates: the read-side projection can briefly return the pre-update values right after a
// successful PUT, which would otherwise make Terraform's post-apply refresh plan non-empty.
func readMappingRuleUntilConsistent(ctx context.Context, client *camunda.ClientWithResponses, mappingRuleId, expectedClaimName, expectedClaimValue, expectedName string) (*camunda.GetMappingRuleResponse, error) {
	return waitForConsistency(ctx, fmt.Sprintf("mapping rule %q", mappingRuleId), func() (*camunda.GetMappingRuleResponse, bool, error) {
		readResp, err := client.GetMappingRuleWithResponse(ctx, mappingRuleId)
		if err != nil {
			return nil, false, err
		}

		if readResp.StatusCode() != http.StatusOK || readResp.JSON200 == nil {
			return readResp, false, nil
		}

		consistent := readResp.JSON200.ClaimName == expectedClaimName &&
			readResp.JSON200.ClaimValue == expectedClaimValue &&
			readResp.JSON200.Name == expectedName

		return readResp, consistent, nil
	})
}
