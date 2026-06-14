// Copyright (c) Ippon
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/ippontech/terraform-provider-anthropic/internal/providerdata"
)

// providerSchema returns the provider's configured schema for building test configs.
func providerSchema(t *testing.T) provider.SchemaResponse {
	t.Helper()
	var resp provider.SchemaResponse
	New("test")().Schema(context.Background(), provider.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %v", resp.Diagnostics)
	}
	return resp
}

// awsObjectType mirrors the nested `aws` attribute object type.
func awsObjectType() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"api_key":      tftypes.String,
			"region":       tftypes.String,
			"workspace_id": tftypes.String,
			"profile":      tftypes.String,
			"base_url":     tftypes.String,
		},
	}
}

func providerObjectType() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"api_key":       tftypes.String,
			"admin_api_key": tftypes.String,
			"base_url":      tftypes.String,
			"aws":           awsObjectType(),
		},
	}
}

// strVal returns a known string value, or null when s is the empty string.
func strVal(s string) tftypes.Value {
	if s == "" {
		return tftypes.NewValue(tftypes.String, nil)
	}
	return tftypes.NewValue(tftypes.String, s)
}

// configValues holds the per-attribute config inputs for a test case.
type configValues struct {
	apiKey      string
	adminAPIKey string
	baseURL     string
	aws         *awsConfigValues
}

type awsConfigValues struct {
	apiKey      string
	region      string
	workspaceID string
	profile     string
	baseURL     string
}

func buildConfig(t *testing.T, vals configValues) tfsdk.Config {
	t.Helper()

	awsVal := tftypes.NewValue(awsObjectType(), nil)
	if vals.aws != nil {
		awsVal = tftypes.NewValue(awsObjectType(), map[string]tftypes.Value{
			"api_key":      strVal(vals.aws.apiKey),
			"region":       strVal(vals.aws.region),
			"workspace_id": strVal(vals.aws.workspaceID),
			"profile":      strVal(vals.aws.profile),
			"base_url":     strVal(vals.aws.baseURL),
		})
	}

	raw := tftypes.NewValue(providerObjectType(), map[string]tftypes.Value{
		"api_key":       strVal(vals.apiKey),
		"admin_api_key": strVal(vals.adminAPIKey),
		"base_url":      strVal(vals.baseURL),
		"aws":           awsVal,
	})

	return tfsdk.Config{Raw: raw, Schema: providerSchema(t).Schema}
}

func configure(t *testing.T, vals configValues) *provider.ConfigureResponse {
	t.Helper()
	resp := &provider.ConfigureResponse{}
	New("test")().Configure(
		context.Background(),
		provider.ConfigureRequest{Config: buildConfig(t, vals)},
		resp,
	)
	return resp
}

// clearEnv blanks the credential env vars so a developer's shell environment
// can't leak into Configure's env-fallback resolution during the test.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_ADMIN_API_KEY",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_AWS_API_KEY",
		"ANTHROPIC_AWS_WORKSPACE_ID",
		"ANTHROPIC_AWS_BASE_URL",
		"AWS_REGION",
		"AWS_DEFAULT_REGION",
	} {
		t.Setenv(k, "")
	}
}

func TestConfigureFirstPartyAPIKey(t *testing.T) {
	clearEnv(t)

	resp := configure(t, configValues{apiKey: "sk-test"})

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok {
		t.Fatalf("ResourceData is not *ProviderData: %T", resp.ResourceData)
	}
	if pd.Client == nil {
		t.Error("expected standard Client to be set")
	}
	if pd.AdminClient != nil {
		t.Error("expected AdminClient to be nil when only api_key is set")
	}
}

func TestConfigureFirstPartyBaseURL(t *testing.T) {
	clearEnv(t)

	resp := configure(t, configValues{
		apiKey:      "sk-test",
		adminAPIKey: "sk-admin",
		baseURL:     "https://proxy.example.com",
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd := resp.ResourceData.(*providerdata.ProviderData)
	if pd.Client == nil {
		t.Error("expected standard Client to be set")
	}
	if pd.AdminClient == nil {
		t.Fatal("expected AdminClient to be set")
	}
	if pd.AdminClient.BaseURL != "https://proxy.example.com" {
		t.Errorf("admin BaseURL = %q, want override", pd.AdminClient.BaseURL)
	}
}

func TestConfigureFirstPartyMissingKeys(t *testing.T) {
	clearEnv(t)

	resp := configure(t, configValues{})

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a Missing API Key error")
	}
}

func TestConfigureAWSAPIKey(t *testing.T) {
	clearEnv(t)

	resp := configure(t, configValues{
		aws: &awsConfigValues{
			apiKey:      "aws-key",
			region:      "us-west-2",
			workspaceID: "wrkspc_test",
		},
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd := resp.ResourceData.(*providerdata.ProviderData)
	if pd.Client == nil {
		t.Error("expected standard Client to be set on AWS path")
	}
	if pd.AdminClient != nil {
		t.Error("expected AdminClient to be nil on AWS path")
	}
}

func TestConfigureAWSMissingRegion(t *testing.T) {
	clearEnv(t)

	resp := configure(t, configValues{
		aws: &awsConfigValues{
			apiKey:      "aws-key",
			workspaceID: "wrkspc_test",
		},
	})

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when AWS region is missing")
	}
}

func TestConfigureAWSMissingWorkspaceID(t *testing.T) {
	clearEnv(t)

	resp := configure(t, configValues{
		aws: &awsConfigValues{
			apiKey: "aws-key",
			region: "us-west-2",
		},
	})

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when AWS workspace_id is missing")
	}
}

func TestConfigureAWSConflictsWithFirstParty(t *testing.T) {
	clearEnv(t)

	resp := configure(t, configValues{
		apiKey: "sk-test",
		aws: &awsConfigValues{
			apiKey:      "aws-key",
			region:      "us-west-2",
			workspaceID: "wrkspc_test",
		},
	})

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a conflict error when both api_key and aws block are set")
	}
}

func TestConfigureAWSWarnsOnAdminKey(t *testing.T) {
	clearEnv(t)

	resp := configure(t, configValues{
		adminAPIKey: "sk-admin",
		aws: &awsConfigValues{
			apiKey:      "aws-key",
			region:      "us-west-2",
			workspaceID: "wrkspc_test",
		},
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() == 0 {
		t.Error("expected a warning that the admin key is ignored on AWS")
	}
	pd := resp.ResourceData.(*providerdata.ProviderData)
	if pd.AdminClient != nil {
		t.Error("expected AdminClient to remain nil on AWS path")
	}
}
