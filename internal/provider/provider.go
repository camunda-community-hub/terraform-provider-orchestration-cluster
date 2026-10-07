package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	camunda "github.com/camunda-community-hub/terraform-provider-orchestration-cluster/pkg/camunda/8.9"
)

// Ensure CamundaClusterProvider satisfies various provider interfaces.
var _ provider.Provider = &CamundaClusterProvider{}

// CamundaClusterProvider defines the provider implementation.
type CamundaClusterProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// CamundaClusterBasicAuthProviderModel describes the provider data model for authenticating with HTTP basic auth.
type CamundaClusterBasicAuthProviderModel struct {
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

// CamundaClusterOIDCAuthProviderModel describes the provider data model for authenticating with OIDC.
type CamundaClusterOIDCAuthProviderModel struct {
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
	LoginURL     types.String `tfsdk:"login_url"`
	Audience     types.String `tfsdk:"audience"`
}

// CamundaClusterProviderModel describes the general provider data model.
type CamundaClusterProviderModel struct {
	BasicAuth *CamundaClusterBasicAuthProviderModel `tfsdk:"basic_auth"`
	OIDC      *CamundaClusterOIDCAuthProviderModel  `tfsdk:"oidc"`
	URL       types.String                          `tfsdk:"url"`

	ConsistencyTimeout types.String `tfsdk:"consistency_timeout"`
}

func (p *CamundaClusterProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "camundacluster"
	resp.Version = p.version
}

func (p *CamundaClusterProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"basic_auth": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"username": schema.StringAttribute{
						MarkdownDescription: "The HTTP Basic Auth username.",
						Required:            true,
					},
					"password": schema.StringAttribute{
						MarkdownDescription: "The HTTP Basic Auth password.",
						Required:            true,
						Sensitive:           true,
					},
				},
				Optional: true,
			},
			"oidc": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"audience": schema.StringAttribute{
						MarkdownDescription: "The audience for the token.",
						Required:            true,
					},
					"client_id": schema.StringAttribute{
						MarkdownDescription: "The client ID",
						Required:            true,
					},
					"client_secret": schema.StringAttribute{
						MarkdownDescription: "The client Secret",
						Required:            true,
						Sensitive:           true,
					},
					"login_url": schema.StringAttribute{
						MarkdownDescription: "The URL of the authentication server.",
						Required:            true,
					},
				},
				Optional: true,
			},
			"consistency_timeout": schema.StringAttribute{
				MarkdownDescription: "How long resources and data sources of this provider instance wait for the cluster's eventually consistent read side to reflect a change, as a Go duration string such as `30s` or `2m`. " +
					"Raise it for slow clusters; lower it to get faster feedback when a data source looks up something that does not exist. " +
					"Must be between 5s and 10m. Defaults to `30s`.",
				Optional:   true,
				Validators: []validator.String{consistencyTimeoutValidator{}},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "The URL of the Camunda cluster API.",
				Required:            true,
			},
		},
	}
}

func (p *CamundaClusterProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data CamundaClusterProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.ConsistencyTimeout.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("consistency_timeout"),
			"Unknown consistency_timeout",
			"The provider cannot be configured with a consistency_timeout that is not known until apply.",
		)
		return
	}

	consistencyTimeout, err := consistencyTimeoutFromConfig(data.ConsistencyTimeout)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("consistency_timeout"), "Invalid consistency_timeout", err.Error())
		return
	}

	opts := []camunda.ClientOption{
		camunda.WithHTTPClient(newLoggingHTTPClient(&http.Client{})),
	}

	if data.BasicAuth != nil {
		tflog.Trace(ctx, "will configure the Camunda client with basic auth")
		opt := camunda.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			req.SetBasicAuth(
				data.BasicAuth.Username.ValueString(),
				data.BasicAuth.Password.ValueString(),
			)
			return nil
		})
		opts = append(opts, opt)
	} else if data.OIDC != nil {
		tflog.Trace(ctx, "will configure the Camunda client with Bearer token")
		token, err := getAuthToken(
			data.OIDC.LoginURL.ValueString(),
			data.OIDC.Audience.ValueString(),
			data.OIDC.ClientID.ValueString(),
			data.OIDC.ClientSecret.ValueString(),
		)
		if err != nil {
			resp.Diagnostics.AddError(
				"Authentication Failed",
				fmt.Sprintf("Unable to authenticate with the provided credentials: %s", err),
			)
			return
		}

		opt := camunda.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		})

		opts = append(opts, opt)
	} else {
		tflog.Trace(ctx, "no authentication configured for the Camunda client, will only make unauthenticated requests")
	}

	var client *camunda.ClientWithResponses
	client, err = camunda.NewClientWithResponses(data.URL.ValueString(), opts...)
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Creation Failed",
			fmt.Sprintf("Unable to create Camunda client: %s", err),
		)
		return
	}

	registerConsistencyTimeout(client, consistencyTimeout)

	resp.DataSourceData = client
	resp.ResourceData = client
}

func getAuthToken(loginUrl, audience, clientID, clientSecret string) (string, error) {

	params := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"audience":      {audience},
	}

	res, err := http.PostForm(loginUrl, params)
	if err != nil {
		return "", err
	}

	if res.StatusCode != 200 {
		return "", fmt.Errorf("failed to authenticate: %s", res.Status)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	type OAuthTokenResponse struct {
		AccessToken string `json:"access_token"`
	}
	var tokenResponse OAuthTokenResponse
	err = json.Unmarshal(body, &tokenResponse)
	if err != nil {
		return "", err
	}

	return tokenResponse.AccessToken, nil
}

func (p *CamundaClusterProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewMappingRuleResource,
		NewUserResource,
		NewTenantResource,
		NewClusterVariableResource,
		NewGroupResource,
		NewRoleResource,
		NewAuthorizationResource,
		NewGroupMemberUserResource,
		NewGroupMemberClientResource,
		NewRoleMemberUserResource,
		NewRoleMemberClientResource,
		NewRoleMemberGroupResource,
		NewTenantMemberClientResource,
		NewTenantMemberUserResource,
		NewTenantMemberGroupResource,
		NewTenantMemberRoleResource,
		NewTenantMemberMappingRuleResource,
		NewGroupMemberMappingRuleResource,
		NewRoleMemberMappingRuleResource,
	}
}

func (p *CamundaClusterProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClusterTopologyDataSource,
		NewUserDataSource,
		NewGroupDataSource,
		NewMappingRuleDataSource,
		NewAuthorizationDataSource,
		NewRoleDataSource,
		NewTenantDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CamundaClusterProvider{
			version: version,
		}
	}
}
