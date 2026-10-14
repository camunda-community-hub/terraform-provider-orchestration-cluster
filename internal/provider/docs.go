package provider

import (
	"fmt"
	"strings"
)

// camundaDocsAdminURL is the base of the Camunda documentation pages that describe the
// Orchestration Cluster Admin concepts. It is deliberately unversioned, so links always
// point at the current documentation.
const camundaDocsAdminURL = "https://docs.camunda.io/docs/components/admin/"

// docPage is a Camunda documentation page below camundaDocsAdminURL.
type docPage struct {
	// label is the plural noun used as link text, e.g. "groups".
	label string
	// slug is the page's path segment below camundaDocsAdminURL.
	slug string
}

var (
	docUsers          = docPage{"users", "user"}
	docGroups         = docPage{"groups", "group"}
	docRoles          = docPage{"roles", "role"}
	docTenants        = docPage{"tenants", "tenant"}
	docAuthorizations = docPage{"authorizations", "authorization"}
	docMappingRules   = docPage{"mapping rules", "mapping-rules"}
	docClients        = docPage{"clients", "client"}
	docClusterVars    = docPage{"cluster variables", "cluster-variables"}
	docClusterAdmin   = docPage{"cluster administration", "cluster-admin"}
)

// docPageByLabel maps the owner and member labels used by membershipDef to their page.
var docPageByLabel = map[string]docPage{
	"user":         docUsers,
	"group":        docGroups,
	"role":         docRoles,
	"tenant":       docTenants,
	"client":       docClients,
	"mapping_rule": docMappingRules,
}

// docsLinkMarkdown returns a sentence linking to the given Camunda documentation pages,
// to be appended to a resource or data source MarkdownDescription.
func docsLinkMarkdown(pages ...docPage) string {
	links := make([]string, len(pages))
	for i, p := range pages {
		links[i] = fmt.Sprintf("[%s](%s%s/)", p.label, camundaDocsAdminURL, p.slug)
	}

	return "See the Camunda documentation on " + strings.Join(links, " and ") + "."
}
