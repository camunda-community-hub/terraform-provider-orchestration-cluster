package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
)

// membershipPageSize is the page size requested once a search has to continue past its first page.
const membershipPageSize = int32(100)

// membershipResponse is the status code and raw body of an assign, unassign or search call.
type membershipResponse struct {
	Status int
	Body   []byte
}

// membershipDef describes one "owner has member" assignment resource, such as a client
// assigned to a tenant. Everything the assignment resources share (schema, create, read,
// delete, composite import ID, drift handling) is implemented once in membershipResource and
// parameterised by this definition.
type membershipDef struct {
	// typeSuffix is appended to the provider name, e.g. "tenant_member_client".
	typeSuffix string
	// ownerLabel and memberLabel are the lower-case nouns used in descriptions and messages,
	// e.g. "tenant" and "client". The attributes are named <label>_id, so memberLabel may contain
	// underscores ("mapping_rule").
	ownerLabel, memberLabel string
	// memberDescription overrides the default "The ID of the <memberLabel>." attribute description.
	memberDescription string

	assign   func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error)
	unassign func(ctx context.Context, c *camunda.ClientWithResponses, ownerId, memberId string) (membershipResponse, error)
	search   func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string, body camunda.SearchQueryRequest) (membershipResponse, error)
	// decodePage extracts the member IDs, the end cursor and the exact total number of matching
	// members (-1 if unknown or capped) from a search response body.
	decodePage func(body []byte) (ids []string, endCursor string, exactTotal int64, err error)
}

func (d membershipDef) ownerAttr() string  { return d.ownerLabel + "_id" }
func (d membershipDef) memberAttr() string { return d.memberLabel + "_id" }

// memberName is memberLabel as prose, e.g. "mapping rule".
func (d membershipDef) memberName() string { return strings.ReplaceAll(d.memberLabel, "_", " ") }

// decodeMembershipPage builds a membershipDef.decodePage for a search result whose items are
// of type T, with idOf extracting the member ID from an item.
func decodeMembershipPage[T any](idOf func(T) string) func([]byte) ([]string, string, int64, error) {
	return func(body []byte) ([]string, string, int64, error) {
		var page struct {
			Items []T `json:"items"`
			Page  struct {
				EndCursor         *string `json:"endCursor"`
				TotalItems        *int64  `json:"totalItems"`
				HasMoreTotalItems bool    `json:"hasMoreTotalItems"`
			} `json:"page"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, "", -1, fmt.Errorf("decode: %w", err)
		}
		ids := make([]string, len(page.Items))
		for i, item := range page.Items {
			ids[i] = idOf(item)
		}
		cursor := ""
		if page.Page.EndCursor != nil {
			cursor = *page.Page.EndCursor
		}
		total := int64(-1)
		if page.Page.TotalItems != nil && !page.Page.HasMoreTotalItems {
			total = *page.Page.TotalItems
		}
		return ids, cursor, total, nil
	}
}

// contains pages through all members of the owner and reports whether memberId is among them.
// It returns an error with message "not_found" if the owner itself is not found.
//
// Pagination stops as soon as the member is found, a page has no items, the end cursor is
// empty, the end cursor did not advance, or the exact totalItems has been consumed.
// hasMoreTotalItems is not a next-page flag: it only says that totalItems is a capped lower
// bound, so totalItems is trusted only when it is false.
func (d membershipDef) contains(ctx context.Context, client *camunda.ClientWithResponses, ownerId, memberId string) (bool, error) {
	var cursor string
	var seen int64
	for {
		body := camunda.SearchQueryRequest{}
		// CursorForwardPagination.After has no `omitempty` json tag, so leaving it at its
		// zero value would still serialize an explicit `"after":""`, which the server
		// rejects as a malformed cursor (HTTP 500) on the very first page. Only set Page
		// at all once there is an actual cursor to send; the server applies its own
		// default paging behavior when the field is omitted entirely.
		if cursor != "" {
			limit := membershipPageSize
			sqpr := camunda.SearchQueryPageRequest{}
			if err := sqpr.FromCursorForwardPagination(camunda.CursorForwardPagination{Limit: &limit, After: cursor}); err != nil {
				return false, fmt.Errorf("encoding cursor: %w", err)
			}
			body.Page = &sqpr
		}

		searchResp, err := d.search(ctx, client, ownerId, body)
		if err != nil {
			return false, err
		}
		if searchResp.Status == http.StatusNotFound {
			return false, fmt.Errorf("not_found")
		}
		if searchResp.Status != http.StatusOK {
			return false, fmt.Errorf("HTTP %d", searchResp.Status)
		}

		ids, endCursor, total, err := d.decodePage(searchResp.Body)
		if err != nil {
			return false, err
		}
		for _, id := range ids {
			if id == memberId {
				return true, nil
			}
		}

		seen += int64(len(ids))
		if len(ids) == 0 || endCursor == "" || endCursor == cursor || (total >= 0 && seen >= total) {
			return false, nil
		}
		cursor = endCursor
	}
}

var _ resource.Resource = &membershipResource{}
var _ resource.ResourceWithImportState = &membershipResource{}

// membershipResource implements an assignment resource for the given definition.
type membershipResource struct {
	def    membershipDef
	client *camunda.ClientWithResponses
}

func newMembershipResource(def membershipDef) resource.Resource {
	return &membershipResource{def: def}
}

func (r *membershipResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.def.typeSuffix
}

func (r *membershipResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	d := r.def
	memberDescription := d.memberDescription
	if memberDescription == "" {
		memberDescription = fmt.Sprintf("The ID of the %s.", d.memberName())
	}
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("Assigns a %s to a Camunda cluster %s", d.memberName(), d.ownerLabel),

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf("Composite ID of the assignment (percent-encoded `%[1]s/%[2]s`). Also used as the import ID: `terraform import camundacluster_%[3]s.example <%[1]s>/<%[2]s>`, with each component percent-encoded if it contains a `/`.", d.ownerAttr(), d.memberAttr(), d.typeSuffix),
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			d.ownerAttr(): schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf("The ID of the %s.", d.ownerLabel),
				Required:            true,
				PlanModifiers:       requiresReplace,
			},
			d.memberAttr(): schema.StringAttribute{
				MarkdownDescription: memberDescription,
				Required:            true,
				PlanModifiers:       requiresReplace,
			},
		},
	}
}

func (r *membershipResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *membershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	d := r.def
	var ownerId, memberId types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(d.ownerAttr()), &ownerId)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(d.memberAttr()), &memberId)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.assign(ctx, r.client, ownerId.ValueString(), memberId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to assign %s to %s, got error: %s", d.memberName(), d.ownerLabel, err))
		return
	}

	// 409 means the assignment already exists: adopt it.
	if apiResp.Status != http.StatusNoContent && apiResp.Status != http.StatusCreated && apiResp.Status != http.StatusOK && apiResp.Status != http.StatusConflict {
		resp.Diagnostics.AddError("Assignment Error", fmt.Sprintf("Error while assigning %s to %s, got HTTP error: %d: %s", d.memberName(), d.ownerLabel, apiResp.Status, apiResp.Body))
		return
	}

	// Persist state from the plan plus the composite ID before polling for read
	// consistency, so a polling timeout or transport error doesn't orphan the assignment
	// the API already made.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), encodeMembershipId(ownerId.ValueString(), memberId.ValueString()))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(d.ownerAttr()), ownerId)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(d.memberAttr()), memberId)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err = waitForConsistency(ctx, r.client, fmt.Sprintf("%s %q %s %q assignment", d.ownerLabel, ownerId.ValueString(), d.memberName(), memberId.ValueString()), func() (bool, bool, error) {
		found, err := d.contains(ctx, r.client, ownerId.ValueString(), memberId.ValueString())
		if err != nil {
			if err.Error() == "not_found" {
				// the owner itself isn't found yet either — treat as pending, same reasoning
				return false, false, nil
			}
			return false, false, err
		}
		return found, found, nil
	})
	if err != nil {
		resp.Diagnostics.AddWarning("Consistency Check Failed", fmt.Sprintf("%s %q was assigned to %s %q but could not be confirmed yet: %s. State was saved from the assignment response; a later refresh will pick up any drift.", capitalize(d.memberName()), memberId.ValueString(), d.ownerLabel, ownerId.ValueString(), err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created %s resource", strings.ReplaceAll(d.typeSuffix, "_", " ")))
}

func (r *membershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	d := r.def
	var ownerId, memberId types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(d.ownerAttr()), &ownerId)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(d.memberAttr()), &memberId)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := d.contains(ctx, r.client, ownerId.ValueString(), memberId.ValueString())
	if err != nil {
		if err.Error() == "not_found" {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Error while reading %s %ss: %s", d.ownerLabel, d.memberName(), err))
		return
	}

	if !found {
		resp.State.RemoveResource(ctx)
	}
}

func (r *membershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	d := r.def
	ownerId, memberId, err := decodeMembershipId(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected import ID in the format <%s>/<%s>, got: %q (%s)", d.ownerAttr(), d.memberAttr(), req.ID, err))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(d.ownerAttr()), ownerId)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(d.memberAttr()), memberId)...)
}

func (r *membershipResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// No mutable state; all changes trigger replace via RequiresReplace
}

func (r *membershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	d := r.def
	var ownerId, memberId types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(d.ownerAttr()), &ownerId)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(d.memberAttr()), &memberId)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.unassign(ctx, r.client, ownerId.ValueString(), memberId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to unassign %s from %s, got error: %s", d.memberName(), d.ownerLabel, err))
		return
	}

	// Already gone
	if apiResp.Status == http.StatusNotFound {
		return
	}

	if apiResp.Status != http.StatusNoContent && apiResp.Status != http.StatusOK {
		resp.Diagnostics.AddError("Delete Error", fmt.Sprintf("Error while unassigning %s from %s, got HTTP error: %d: %s", d.memberName(), d.ownerLabel, apiResp.Status, apiResp.Body))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("deleted %s resource", strings.ReplaceAll(d.typeSuffix, "_", " ")))
}

// encodeMembershipId joins the percent-encoded owner and member IDs with "/".
func encodeMembershipId(ownerId, memberId string) string {
	return url.PathEscape(ownerId) + "/" + url.PathEscape(memberId)
}

// decodeMembershipId splits a composite ID created by encodeMembershipId. The two components
// are percent-encoded before being joined, so splitting on the first "/" is unambiguous even
// if an ID itself contains a "/": that "/" only ever appears escaped as "%2F". An ID with more
// than one unescaped "/" is therefore malformed and rejected.
func decodeMembershipId(id string) (ownerId, memberId string, err error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("missing component")
	}
	if strings.Contains(parts[1], "/") {
		return "", "", fmt.Errorf("unescaped \"/\" in second component")
	}
	if ownerId, err = url.PathUnescape(parts[0]); err != nil {
		return "", "", fmt.Errorf("decoding first component: %w", err)
	}
	if memberId, err = url.PathUnescape(parts[1]); err != nil {
		return "", "", fmt.Errorf("decoding second component: %w", err)
	}
	return ownerId, memberId, nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
