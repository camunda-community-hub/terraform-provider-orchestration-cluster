package provider

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.10"
)

// membersKind is one member type of a membersDataSource: the computed attribute that holds
// the member IDs and the search that lists them.
type membersKind struct {
	// label is the lower-case member noun, e.g. "mapping_rule". The attribute is <label>_ids.
	label  string
	search memberSearch
}

func (k membersKind) attr() string { return k.label + "_ids" }

func (k membersKind) name() string { return strings.ReplaceAll(k.label, "_", " ") }

// membersDataSourceDef describes a data source that lists the members of one owner entity,
// grouped by member type. Everything the three members data sources share (schema, paging,
// eventual consistency handling) is implemented once in membersDataSource.
type membersDataSourceDef struct {
	// typeSuffix is appended to the provider name, e.g. "group_members".
	typeSuffix string
	// ownerLabel is the lower-case owner noun, e.g. "group". The lookup attribute is <ownerLabel>_id.
	ownerLabel string
	ownerPage  docPage
	// getOwner returns the status code of the owner's GET endpoint. The member searches answer
	// 200 with an empty list for an owner that does not exist, so existence is checked separately.
	getOwner func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string) (int, error)
	kinds    []membersKind
}

func (d membersDataSourceDef) ownerAttr() string { return d.ownerLabel + "_id" }

var _ datasource.DataSource = &membersDataSource{}

type membersDataSource struct {
	def    membersDataSourceDef
	client *camunda.ClientWithResponses
}

func newMembersDataSource(def membersDataSourceDef) datasource.DataSource {
	return &membersDataSource{def: def}
}

func (d *membersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.def.typeSuffix
}

func (d *membersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	def := d.def
	attributes := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: fmt.Sprintf("The ID of the %s (the same as `%s`).", def.ownerLabel, def.ownerAttr()),
			Computed:            true,
		},
		def.ownerAttr(): schema.StringAttribute{
			MarkdownDescription: fmt.Sprintf("The ID of the %s whose members are listed. It must exist.", def.ownerLabel),
			Required:            true,
		},
	}
	pages := []docPage{def.ownerPage}
	for _, k := range def.kinds {
		attributes[k.attr()] = schema.SetAttribute{
			MarkdownDescription: fmt.Sprintf("The IDs of the %ss assigned to the %s. Empty if there are none.", k.name(), def.ownerLabel),
			ElementType:         types.StringType,
			Computed:            true,
		}
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("Lists the members of a %s inside a Camunda cluster, grouped by member type, "+
			"including members that Terraform does not manage. An empty member list is a valid result; a %s that does not exist is an error. "+
			"Clients exist only as members of groups, roles and tenants, so they are listed here and have no data source of their own. %s",
			def.ownerLabel, def.ownerLabel, docsLinkMarkdown(pages...)),
		Attributes: attributes,
	}
}

func (d *membersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// readAll lists the members of every kind, each sorted. It returns an error with message
// "not_found" if the owner is not found.
func (d *membersDataSource) readAll(ctx context.Context, ownerId string) (map[string][]string, error) {
	lists := make(map[string][]string, len(d.def.kinds))
	for _, k := range d.def.kinds {
		ids, err := k.search.all(ctx, d.client, ownerId)
		if err != nil {
			return nil, err
		}
		slices.Sort(ids)
		lists[k.attr()] = slices.Compact(ids)
	}
	return lists, nil
}

func (d *membersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	def := d.def
	var ownerId types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(def.ownerAttr()), &ownerId)...)
	if resp.Diagnostics.HasError() {
		return
	}
	owner := ownerId.ValueString()

	// The member searches are eventually consistent: members assigned earlier in the same apply
	// may not be listed yet, and so may not the owner itself. Accept the lists once two
	// consecutive reads are identical. This narrows, but cannot fully close, the race where a
	// projection lags for longer than one poll interval. A missing owner is treated as pending
	// and reported as not found if it never appears.
	var previous map[string][]string
	ownerMissing := false
	lists, err := waitForConsistency(ctx, d.client, fmt.Sprintf("%s %q members", def.ownerLabel, owner), func() (map[string][]string, bool, error) {
		ownerMissing = false
		status, err := def.getOwner(ctx, d.client, owner)
		if err != nil {
			return nil, false, err
		}
		if status == http.StatusNotFound {
			ownerMissing = true
			previous = nil
			return nil, false, nil
		}
		if status != http.StatusOK {
			return nil, false, fmt.Errorf("HTTP %d while reading the %s", status, def.ownerLabel)
		}

		current, err := d.readAll(ctx, owner)
		if err != nil {
			if err.Error() == "not_found" {
				ownerMissing = true
				previous = nil
				return nil, false, nil
			}
			return nil, false, err
		}
		stable := previous != nil && maps.EqualFunc(previous, current, slices.Equal[[]string])
		previous = current
		return current, stable, nil
	})
	if err != nil {
		if ownerMissing {
			resp.Diagnostics.AddError("Not Found", fmt.Sprintf("No %s found with ID '%s': %s", def.ownerLabel, owner, err))
			return
		}
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Unable to list the members of %s '%s': %s", def.ownerLabel, owner, err))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), owner)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(def.ownerAttr()), owner)...)
	for _, k := range def.kinds {
		elements := make([]attr.Value, len(lists[k.attr()]))
		for i, id := range lists[k.attr()] {
			elements[i] = types.StringValue(id)
		}
		set, diags := types.SetValue(types.StringType, elements)
		resp.Diagnostics.Append(diags...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(k.attr()), set)...)
	}

	tflog.Trace(ctx, fmt.Sprintf("read %s data source", def.typeSuffix))
}

// memberSearchOf is the search of an assignment definition as a memberSearch.
func memberSearchOf(d membershipDef) memberSearch {
	return memberSearch{search: d.search, decodePage: d.decodePage}
}

// groupRoleSearch lists the roles assigned to a group. Roles are assigned to groups from the
// role side (orchestration_cluster_role_member_group), so there is no assignment definition.
var groupRoleSearch = memberSearch{
	search: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string, body camunda.SearchQueryRequest) (membershipResponse, error) {
		r, err := c.SearchRolesForGroupWithResponse(ctx, ownerId, body)
		if err != nil {
			return membershipResponse{}, err
		}
		return membershipResponse{r.StatusCode(), r.Body}, nil
	},
	decodePage: decodeMembershipPage(func(r camunda.RoleResult) string { return r.RoleId }),
}

func NewGroupMembersDataSource() datasource.DataSource {
	return newMembersDataSource(membersDataSourceDef{
		typeSuffix: "group_members",
		ownerLabel: "group",
		ownerPage:  docGroups,
		getOwner: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string) (int, error) {
			r, err := c.GetGroupWithResponse(ctx, ownerId)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		kinds: []membersKind{
			{"user", memberSearchOf(groupMemberUser)},
			{"client", memberSearchOf(groupMemberClient)},
			{"mapping_rule", memberSearchOf(groupMemberMappingRule)},
			{"role", groupRoleSearch},
		},
	})
}

func NewRoleMembersDataSource() datasource.DataSource {
	return newMembersDataSource(membersDataSourceDef{
		typeSuffix: "role_members",
		ownerLabel: "role",
		ownerPage:  docRoles,
		getOwner: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string) (int, error) {
			r, err := c.GetRoleWithResponse(ctx, ownerId)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		kinds: []membersKind{
			{"user", memberSearchOf(roleMemberUser)},
			{"client", memberSearchOf(roleMemberClient)},
			{"group", memberSearchOf(roleMemberGroup)},
			{"mapping_rule", memberSearchOf(roleMemberMappingRule)},
		},
	})
}

func NewTenantMembersDataSource() datasource.DataSource {
	return newMembersDataSource(membersDataSourceDef{
		typeSuffix: "tenant_members",
		ownerLabel: "tenant",
		ownerPage:  docTenants,
		getOwner: func(ctx context.Context, c *camunda.ClientWithResponses, ownerId string) (int, error) {
			r, err := c.GetTenantWithResponse(ctx, ownerId)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		kinds: []membersKind{
			{"user", memberSearchOf(tenantMemberUser)},
			{"client", memberSearchOf(tenantMemberClient)},
			{"group", memberSearchOf(tenantMemberGroup)},
			{"role", memberSearchOf(tenantMemberRole)},
			{"mapping_rule", memberSearchOf(tenantMemberMappingRule)},
		},
	})
}
