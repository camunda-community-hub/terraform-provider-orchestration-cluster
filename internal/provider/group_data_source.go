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

var _ datasource.DataSource = &GroupDataSource{}

func NewGroupDataSource() datasource.DataSource {
	return &GroupDataSource{}
}

type GroupDataSource struct {
	client *camunda.ClientWithResponses
}

type GroupDataSourceModel struct {
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (d *GroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *GroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Camunda cluster group by name.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique ID of the group.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The display name of the group to look up. Must match exactly one group.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the group.",
				Computed:            true,
			},
		},
	}
}

func (d *GroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// groupSearchByNameRequest is sent as a raw JSON body to POST /groups/search instead of via the
// generated SearchGroupsJSONRequestBody. The OpenAPI spec defines that request as an allOf of the
// generic SearchQueryRequest plus sibling `filter`/`sort` properties, but the generator collapses
// such allOf-with-siblings schemas into a bare alias of the base type, silently dropping `filter`
// (see GroupFilter, which is generated but never referenced). Building the body by hand is the
// only way to actually filter server-side with the current generated client.
type groupSearchByNameRequest struct {
	Filter struct {
		Name string `json:"name"`
	} `json:"filter"`
}

// groupSearchQueryResult mirrors the actual /groups/search response shape. For the same reason
// as groupSearchByNameRequest above, the generated GroupSearchQueryResult type only carries
// pagination info and drops the `items` array, so SearchGroupsResponse.JSON200 can never report
// any results. SearchGroupsResponse.Body still holds the raw bytes, which this type decodes.
type groupSearchQueryResult struct {
	Items []camunda.GroupResult `json:"items"`
}

func (d *GroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data GroupDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()

	// POST /groups/search is eventually consistent: a group created earlier in the same
	// apply may not be searchable yet even though its own create already completed. Poll
	// while the search reports zero matches, rather than converting the first empty
	// response directly into "Not Found" -- but don't retry on 1 (a real match) or on 2+
	// (a genuine ambiguous-name condition, not a consistency issue). The request body is
	// re-marshaled and a fresh reader constructed on every attempt, since an io.Reader
	// can't be replayed after being consumed by a previous attempt.
	items, err := waitForConsistency(ctx, fmt.Sprintf("group named %q", name), func() ([]camunda.GroupResult, bool, error) {
		filterReq := groupSearchByNameRequest{}
		filterReq.Filter.Name = name

		bodyBytes, err := json.Marshal(filterReq)
		if err != nil {
			return nil, false, fmt.Errorf("unable to encode group search request: %w", err)
		}

		apiResp, err := d.client.SearchGroupsWithBodyWithResponse(ctx, "application/json", bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, false, err
		}

		if apiResp.StatusCode() != http.StatusOK {
			return nil, false, fmt.Errorf("got HTTP error: %d: %s", apiResp.StatusCode(), apiResp.Body)
		}

		var result groupSearchQueryResult
		if err := json.Unmarshal(apiResp.Body, &result); err != nil {
			return nil, false, fmt.Errorf("unable to parse group search response: %w", err)
		}

		return result.Items, len(result.Items) > 0, nil
	})
	// waitForConsistency only reports "ready" once at least one match is found, so an error
	// here means the search never returned any match within the timeout -- a real "not
	// found" (or a persistent hard error, whose detail is still included in the message).
	if err != nil {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("No group found with name '%s': %s", name, err))
		return
	}

	if len(items) > 1 {
		resp.Diagnostics.AddError("Ambiguous Lookup", fmt.Sprintf("Name '%s' matched %d groups; group names are not guaranteed unique.", name, len(items)))
		return
	}

	match := items[0]
	data.Id = types.StringValue(match.GroupId)
	data.Name = types.StringValue(match.Name)
	if match.Description != nil {
		data.Description = types.StringValue(*match.Description)
	} else {
		data.Description = types.StringValue("")
	}

	tflog.Trace(ctx, "read group data source")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
