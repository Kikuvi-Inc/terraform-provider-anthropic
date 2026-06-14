// Copyright (c) Ippon
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/aws"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ippontech/terraform-provider-anthropic/internal/admin"
	"github.com/ippontech/terraform-provider-anthropic/internal/providerdata"
	"github.com/ippontech/terraform-provider-anthropic/internal/services/agents"
	"github.com/ippontech/terraform-provider-anthropic/internal/services/apikeys"
	"github.com/ippontech/terraform-provider-anthropic/internal/services/environments"
	"github.com/ippontech/terraform-provider-anthropic/internal/services/messages"
	"github.com/ippontech/terraform-provider-anthropic/internal/services/models"
	"github.com/ippontech/terraform-provider-anthropic/internal/services/skills"
	"github.com/ippontech/terraform-provider-anthropic/internal/services/workspaces"
)

// Ensure AnthropicProvider satisfies various provider interfaces.
var _ provider.Provider = &AnthropicProvider{}

// AnthropicProvider defines the provider implementation.
type AnthropicProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// AnthropicProviderModel describes the provider data model.
type AnthropicProviderModel struct {
	ApiKey      types.String       `tfsdk:"api_key"`
	AdminApiKey types.String       `tfsdk:"admin_api_key"`
	BaseURL     types.String       `tfsdk:"base_url"`
	AWS         *AnthropicAWSModel `tfsdk:"aws"`
}

// AnthropicAWSModel configures the Claude Platform on AWS backend. Its presence
// in the config selects the AWS gateway for the standard and beta API surfaces
// instead of the first-party endpoint.
type AnthropicAWSModel struct {
	ApiKey      types.String `tfsdk:"api_key"`
	Region      types.String `tfsdk:"region"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
	Profile     types.String `tfsdk:"profile"`
	BaseURL     types.String `tfsdk:"base_url"`
}

func (p *AnthropicProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "anthropic"
	resp.Version = p.version
}

func (p *AnthropicProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The Anthropic API key. Can also be set via the ANTHROPIC_API_KEY environment variable.",
			},
			"admin_api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The Anthropic Admin API key for organization management endpoints (workspaces, members). Can also be set via the ANTHROPIC_ADMIN_API_KEY environment variable.",
			},
			"base_url": schema.StringAttribute{
				Optional:    true,
				Description: "Override the base URL for the first-party Anthropic API (e.g. a proxy or gateway). Can also be set via the ANTHROPIC_BASE_URL environment variable. Ignored when the `aws` block is set.",
			},
			"aws": schema.SingleNestedAttribute{
				Optional: true,
				Description: "Configure the Claude Platform on AWS backend. When set, standard and beta API resources " +
					"(messages, models, count_tokens, agents, environments, skills) use the AWS gateway instead of the " +
					"first-party endpoint. Admin API resources (api_key, workspace_member, workspace_rate_limits) are not " +
					"available on AWS and require the first-party `admin_api_key`.",
				Attributes: map[string]schema.Attribute{
					"api_key": schema.StringAttribute{
						Optional:    true,
						Sensitive:   true,
						Description: "API key for the Claude Platform on AWS gateway. Can also be set via the ANTHROPIC_AWS_API_KEY environment variable. When omitted, SigV4 authentication is resolved via the standard AWS credential chain (environment variables, shared credentials file, IAM role).",
					},
					"region": schema.StringAttribute{
						Optional:    true,
						Description: "AWS region the workspace is bound to. Required for AWS. Can also be set via the AWS_REGION environment variable.",
					},
					"workspace_id": schema.StringAttribute{
						Optional:    true,
						Description: "Claude Platform on AWS workspace ID (sent as the anthropic-workspace-id header). Required for AWS. Can also be set via the ANTHROPIC_AWS_WORKSPACE_ID environment variable.",
					},
					"profile": schema.StringAttribute{
						Optional:    true,
						Description: "AWS named profile used to resolve SigV4 credentials via the AWS credential chain.",
					},
					"base_url": schema.StringAttribute{
						Optional:    true,
						Description: "Override the Claude Platform on AWS gateway base URL. Can also be set via the ANTHROPIC_AWS_BASE_URL environment variable. Defaults to https://aws-external-anthropic.{region}.api.aws.",
					},
				},
			},
		},
	}
}

func (p *AnthropicProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data AnthropicProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if !data.ApiKey.IsNull() && !data.ApiKey.IsUnknown() {
		apiKey = data.ApiKey.ValueString()
	}

	adminApiKey := os.Getenv("ANTHROPIC_ADMIN_API_KEY")
	if !data.AdminApiKey.IsNull() && !data.AdminApiKey.IsUnknown() {
		adminApiKey = data.AdminApiKey.ValueString()
	}

	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if !data.BaseURL.IsNull() && !data.BaseURL.IsUnknown() {
		baseURL = data.BaseURL.ValueString()
	}

	pd := &providerdata.ProviderData{}

	if data.AWS != nil {
		// Claude Platform on AWS backend. The standard client is built against
		// the AWS gateway; first-party api_key/base_url do not apply and are
		// ignored. Only conflict on values set EXPLICITLY in HCL — an ambient
		// ANTHROPIC_API_KEY / ANTHROPIC_BASE_URL in the shell or CI runner is not
		// declared intent and must not break an explicit `aws` block.
		if isSet(data.ApiKey) || isSet(data.BaseURL) {
			resp.Diagnostics.AddError(
				"Conflicting backend configuration",
				"The `aws` block selects the Claude Platform on AWS backend for standard and beta resources, "+
					"so `api_key` and `base_url` must not also be set in the provider configuration. "+
					"Remove them, or remove the `aws` block to use the first-party API. "+
					"(Ambient ANTHROPIC_API_KEY / ANTHROPIC_BASE_URL environment variables are ignored on the AWS backend.)",
			)
			return
		}

		client, err := newAWSClient(ctx, data.AWS)
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to configure Claude Platform on AWS client",
				err.Error(),
			)
			return
		}
		pd.Client = client

		// Admin API resources are unavailable on AWS; warn if an admin key was
		// provided rather than failing the whole configuration.
		if adminApiKey != "" {
			resp.Diagnostics.AddWarning(
				"Admin API key ignored on Claude Platform on AWS",
				"Admin API resources (anthropic_api_key, anthropic_workspace_member, anthropic_workspace_rate_limits) "+
					"are not available on Claude Platform on AWS. The configured admin_api_key is ignored.",
			)
		}

		resp.DataSourceData = pd
		resp.ResourceData = pd
		return
	}

	// First-party backend.
	if apiKey == "" && adminApiKey == "" {
		resp.Diagnostics.AddError(
			"Missing API Key",
			"At least one API key must be configured: api_key (ANTHROPIC_API_KEY) for standard resources, "+
				"admin_api_key (ANTHROPIC_ADMIN_API_KEY) for organization management resources, "+
				"or the `aws` block for Claude Platform on AWS.",
		)
		return
	}

	if apiKey != "" {
		opts := []option.RequestOption{option.WithAPIKey(apiKey)}
		if baseURL != "" {
			opts = append(opts, option.WithBaseURL(baseURL))
		}
		client := anthropic.NewClient(opts...)
		pd.Client = &client
	}
	if adminApiKey != "" {
		pd.AdminClient = admin.NewClientWithBaseURL(adminApiKey, baseURL)
	}

	resp.DataSourceData = pd
	resp.ResourceData = pd
}

// newAWSClient builds a standard Anthropic client backed by the Claude Platform
// on AWS gateway. The SDK's *aws.Client and *anthropic.Client share the same
// service types (Messages, Models, Beta, Completions); only the resolved request
// options differ (gateway base URL, SigV4/API-key auth, anthropic-workspace-id
// header). We copy the AWS-resolved options and services into an
// *anthropic.Client so that ProviderData.Client stays *anthropic.Client and every
// resource works unchanged. We deliberately avoid anthropic.NewClient here, which
// would prepend DefaultClientOptions() and leak ANTHROPIC_API_KEY /
// ANTHROPIC_BASE_URL into the AWS path.
func newAWSClient(ctx context.Context, cfg *AnthropicAWSModel) (*anthropic.Client, error) {
	awsClient, err := aws.NewClient(ctx, aws.ClientConfig{
		APIKey:      stringValue(cfg.ApiKey),
		AWSRegion:   stringValue(cfg.Region),
		WorkspaceID: stringValue(cfg.WorkspaceID),
		AWSProfile:  stringValue(cfg.Profile),
		BaseURL:     stringValue(cfg.BaseURL),
	})
	if err != nil {
		return nil, err
	}

	// Copies every field of anthropic.Client. When upgrading the SDK, verify the
	// struct still has exactly these fields — a new field added upstream would be
	// silently dropped here, leaving AWS-backed resources misconfigured.
	return &anthropic.Client{
		Options:     awsClient.Options,
		Completions: awsClient.Completions,
		Messages:    awsClient.Messages,
		Models:      awsClient.Models,
		Beta:        awsClient.Beta,
	}, nil
}

// stringValue returns the underlying string for a known, non-null value, and ""
// otherwise. Empty values let the SDK fall back to its own env-var resolution
// (AWS_REGION, ANTHROPIC_AWS_WORKSPACE_ID, ANTHROPIC_AWS_API_KEY,
// ANTHROPIC_AWS_BASE_URL).
func stringValue(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

// isSet reports whether an attribute was set explicitly in the provider
// configuration (a known, non-null value), as opposed to resolved from an
// environment variable.
func isSet(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown()
}

func (p *AnthropicProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		agents.NewAgentResource,
		apikeys.NewAPIKeyResource,
		environments.NewEnvironmentResource,
		messages.NewMessageResource,
		skills.NewSkillResource,
		skills.NewSkillVersionResource,
		workspaces.NewWorkspaceResource,
		workspaces.NewWorkspaceMemberResource,
	}
}

func (p *AnthropicProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		agents.NewAgentDataSource,
		apikeys.NewAPIKeyDataSource,
		apikeys.NewAPIKeysDataSource,
		agents.NewAgentsDataSource,
		messages.NewCountTokensDataSource,
		environments.NewEnvironmentDataSource,
		environments.NewEnvironmentsDataSource,
		models.NewModelDataSource,
		models.NewModelsDataSource,
		skills.NewSkillDataSource,
		skills.NewSkillVersionDataSource,
		skills.NewSkillVersionsDataSource,
		skills.NewSkillsDataSource,
		workspaces.NewWorkspaceDataSource,
		workspaces.NewWorkspaceMemberDataSource,
		workspaces.NewWorkspaceMembersDataSource,
		workspaces.NewWorkspaceRateLimitsDataSource,
		workspaces.NewWorkspacesDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &AnthropicProvider{
			version: version,
		}
	}
}
