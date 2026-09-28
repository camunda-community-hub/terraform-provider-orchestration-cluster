package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda/terraform-provider-camunda-cluster/pkg/camunda/8.9"
)

var _ datasource.DataSource = &RoleDataSource{}

func NewRoleDataSource() datasource.DataSource {
	return &RoleDataSource{}
}

type RoleDataSource struct {
	client *camunda.ClientWithResponses
}

type RoleDataSourceModel struct {
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (d *RoleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (d *RoleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Camunda cluster role by name.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of the role.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The display name of the role to look up. Must match exactly one role.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the role.",
				Computed:            true,
			},
		},
	}
}

func (d *RoleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*camunda.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *camunda.ClientWithResponses, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

// roleSearchByNameRequest is sent as a raw JSON body to POST /roles/search instead of via the
// generated SearchRolesJSONRequestBody. The OpenAPI spec defines that request as an allOf of the
// generic SearchQueryRequest plus sibling `filter`/`sort` properties, but the generator collapses
// such allOf-with-siblings schemas into a bare alias of the base type, silently dropping `filter`
// (see RoleFilter, which is generated but never referenced). Building the body by hand is the
// only way to actually filter server-side with the current generated client.
type roleSearchByNameRequest struct {
	Filter struct {
		Name string `json:"name"`
	} `json:"filter"`
}

// roleSearchQueryResult mirrors the actual /roles/search response shape. For the same reason as
// roleSearchByNameRequest above, the generated RoleSearchQueryResult type only carries pagination
// info and drops the `items` array, so SearchRolesResponse.JSON200 can never report any results.
// SearchRolesResponse.Body still holds the raw bytes, which this type decodes.
type roleSearchQueryResult struct {
	Items []camunda.RoleResult `json:"items"`
}

func (d *RoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RoleDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()

	// POST /roles/search is eventually consistent: a role created earlier in the same
	// apply may not be searchable yet even though its own create already completed, and
	// two same-named roles can be projected one at a time, so a single nonempty result
	// doesn't prove uniqueness. lastCount tracks the previous poll's result count so the
	// closure below can require it to be stable (unchanged and nonzero) across two
	// consecutive polls before accepting it, rather than trusting the first nonzero
	// count -- this narrows, but can't fully close, the race where a second duplicate
	// lands in the gap between two "stable" polls; there's no uniqueness-guaranteed
	// lookup available from this API to close it completely. hardErr captures any
	// transport/HTTP/decode failure from the closure so it can be distinguished below
	// from a genuine "polled to timeout with zero matches" case; both are reported as an
	// error by waitForConsistency, but only the latter is actually a "not found" signal.
	// The request body is re-marshaled and a fresh reader constructed on every attempt,
	// since an io.Reader can't be replayed after being consumed by a previous attempt.
	lastCount := -1
	var hardErr error
	items, err := waitForConsistency(ctx, fmt.Sprintf("role named %q", name), func() ([]camunda.RoleResult, bool, error) {
		hardErr = nil

		filterReq := roleSearchByNameRequest{}
		filterReq.Filter.Name = name

		bodyBytes, err := json.Marshal(filterReq)
		if err != nil {
			hardErr = fmt.Errorf("unable to encode role search request: %w", err)
			return nil, false, hardErr
		}

		apiResp, err := d.client.SearchRolesWithBodyWithResponse(ctx, "application/json", bytes.NewReader(bodyBytes))
		if err != nil {
			hardErr = err
			return nil, false, hardErr
		}

		if apiResp.StatusCode() != http.StatusOK {
			hardErr = fmt.Errorf("got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body)
			return nil, false, hardErr
		}

		var result roleSearchQueryResult
		if err := json.Unmarshal(apiResp.Body, &result); err != nil {
			hardErr = fmt.Errorf("unable to parse role search response: %w", err)
			return nil, false, hardErr
		}

		count := len(result.Items)
		stable := count > 0 && count == lastCount
		lastCount = count
		return result.Items, stable, nil
	})
	if hardErr != nil {
		resp.Diagnostics.AddError("Search Error", fmt.Sprintf("Unable to search for role named '%s': %s", name, hardErr))
		return
	}
	// With no hard error, an error here means waitForConsistency timed out. lastCount == 0
	// means the last attempt cleanly reported zero matches -- a real "not found". A
	// positive lastCount means matches kept appearing but never stabilized across two
	// consecutive polls, which is not the same as "not found" and must not be reported as
	// one; -1 means the search was cancelled before a first poll completed.
	if err != nil {
		if lastCount == 0 {
			resp.Diagnostics.AddError("Not Found", fmt.Sprintf("No role found with name '%s': %s", name, err))
		} else {
			resp.Diagnostics.AddError("Search Error", fmt.Sprintf("Search for role named '%s' did not stabilize: %s", name, err))
		}
		return
	}

	if len(items) > 1 {
		resp.Diagnostics.AddError("Ambiguous Lookup", fmt.Sprintf("Name '%s' matched %d roles; role names are not guaranteed unique.", name, len(items)))
		return
	}

	match := items[0]
	data.Id = types.StringValue(match.RoleId)
	data.Name = types.StringValue(match.Name)
	data.Description = optionalStringValue(match.Description)

	tflog.Trace(ctx, "read role data source")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
