---
page_title: "Provider: Anthropic"
description: |-
  Use the Anthropic Terraform provider to interact with Anthropic APIs.
---

# Anthropic Provider

The Anthropic provider is used to interact with [Anthropic](https://anthropic.com) APIs.
It allows you to manage and query Anthropic resources such as models.

## Authentication

The provider uses two distinct API keys depending on which resources you manage.

### API Key (`api_key` / `ANTHROPIC_API_KEY`)

Required for all inference resources: `anthropic_message`, `anthropic_agent`, `anthropic_skill`, models, token counting, etc.

Generate one in the [Anthropic Console → API Keys](https://platform.claude.com/settings/keys).

**Environment variable** (recommended):

```bash
export ANTHROPIC_API_KEY="sk-ant-api03-..."
```

**Provider argument**:

```hcl
provider "anthropic" {
  api_key = "sk-ant-api03-..."
}
```

### Admin API Key (`admin_api_key` / `ANTHROPIC_ADMIN_API_KEY`)

Required for organization-management resources: `anthropic_workspace`, workspace members, and workspace rate limits. This is a separate credential scoped to your whole organization rather than a single workspace.

Generate one in the [Anthropic Console → Admin API Keys](https://platform.claude.com/settings/admin-keys).

**Environment variable** (recommended):

```bash
export ANTHROPIC_ADMIN_API_KEY="sk-ant-admin03-..."
```

**Provider argument**:

```hcl
provider "anthropic" {
  admin_api_key = "sk-ant-admin03-..."
}
```

The `admin_api_key` is optional — you only need it when using workspace-related resources.

~> **Warning**: Never hardcode API keys in your Terraform configuration files.
Use environment variables or a secrets manager instead.

## Base URL override (`base_url` / `ANTHROPIC_BASE_URL`)

Override the base URL for the first-party Anthropic API — useful for a corporate proxy, an LLM gateway, or a local mock during testing. Applies to both the standard and admin clients. Ignored when the `aws` block is set.

```hcl
provider "anthropic" {
  base_url = "https://anthropic-proxy.internal.example.com"
}
```

## Claude Platform on AWS (`aws` block)

[Claude Platform on AWS](https://platform.claude.com/docs/en/build-with-claude/claude-platform-on-aws) is Anthropic-operated access to the Claude Developer Platform through AWS, with SigV4 / AWS-IAM authentication, AWS Marketplace billing, and a separate Anthropic organization bound to your AWS account. Setting the `aws` block routes all standard and beta resources through the AWS gateway instead of the first-party endpoint.

The `region` and `workspace_id` fields are required; both can also be supplied via environment variables (`AWS_REGION`, `ANTHROPIC_AWS_WORKSPACE_ID`).

**API-key authentication** (generate the key in the AWS Console under Claude Platform on AWS → API keys):

```hcl
provider "anthropic" {
  aws = {
    api_key      = "..."           # or ANTHROPIC_AWS_API_KEY
    region       = "us-west-2"     # or AWS_REGION
    workspace_id = "wrkspc_..."    # or ANTHROPIC_AWS_WORKSPACE_ID
  }
}
```

**SigV4 authentication** — omit `aws.api_key` and the provider resolves AWS credentials via the standard credential chain (environment variables, shared credentials file, IAM role). Optionally pin a named profile:

```hcl
provider "anthropic" {
  aws = {
    region       = "us-west-2"
    workspace_id = "wrkspc_..."
    profile      = "my-aws-profile"  # optional
  }
}
```

~> **Note**: On Claude Platform on AWS only the standard and beta API surfaces are available — `anthropic_message`, `anthropic_model(s)`, `anthropic_count_tokens`, and the Managed Agents family (`anthropic_agent(s)`, `anthropic_environment(s)`, `anthropic_skill(_version)(s)`). The Admin API resources (`anthropic_api_key(s)`, `anthropic_workspace(s)`, `anthropic_workspace_member(s)`, `anthropic_workspace_rate_limits`) are not available on AWS; a configured `admin_api_key` is ignored with a warning. Setting `api_key` or `base_url` *explicitly* alongside the `aws` block is a configuration error; ambient `ANTHROPIC_API_KEY` / `ANTHROPIC_BASE_URL` environment variables are simply ignored on the AWS backend.

## Cost considerations

~> **Warning**: Some resources call billable Anthropic APIs at `terraform apply` time and can generate unbounded variable cost. `anthropic_message` consumes tokens on every apply, and Managed Agents / Skills (created via `anthropic_agent`, `anthropic_environment`, `anthropic_skill`, `anthropic_skill_version`) are free to manage but billable when invoked at runtime. Combining these with `count`/`for_each` over a large input set, or omitting `max_tokens`, can produce a single apply that costs significantly more than expected.

Each per-resource doc page lists its cost profile in an **API / Auth / Beta header / Cost** header block. Review it before scaling apply-time inference across many instances, and prefer setting an explicit `max_tokens` on every `anthropic_message`.

## Example Usage

```hcl
terraform {
  required_version = ">= 1.0"

  required_providers {
    anthropic = {
      source  = "registry.terraform.io/ippontech/anthropic"
      version = "~> 1.0"
    }
  }
}

# First-party Anthropic API (default). Reads ANTHROPIC_API_KEY from the environment.
provider "anthropic" {}

# Claude Platform on AWS — routes standard and beta resources through the AWS gateway.
# Admin API resources are not available on this backend.
#
# provider "anthropic" {
#   aws = {
#     region       = "us-west-2"  # or AWS_REGION
#     workspace_id = "wrkspc_..." # or ANTHROPIC_AWS_WORKSPACE_ID
#     # api_key   = "..."         # or ANTHROPIC_AWS_API_KEY; omit to use SigV4 via the AWS credential chain
#     # profile   = "my-profile"  # optional named AWS profile for SigV4
#   }
# }
```

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `admin_api_key` (String, Sensitive) The Anthropic Admin API key for organization management endpoints (workspaces, members). Can also be set via the ANTHROPIC_ADMIN_API_KEY environment variable.
- `api_key` (String, Sensitive) The Anthropic API key. Can also be set via the ANTHROPIC_API_KEY environment variable.
- `aws` (Attributes) Configure the Claude Platform on AWS backend. When set, standard and beta API resources (messages, models, count_tokens, agents, environments, skills) use the AWS gateway instead of the first-party endpoint. Admin API resources (api_key, workspace_member, workspace_rate_limits) are not available on AWS and require the first-party `admin_api_key`. (see [below for nested schema](#nestedatt--aws))
- `base_url` (String) Override the base URL for the first-party Anthropic API (e.g. a proxy or gateway). Can also be set via the ANTHROPIC_BASE_URL environment variable. Ignored when the `aws` block is set.

<a id="nestedatt--aws"></a>
### Nested Schema for `aws`

Optional:

- `api_key` (String, Sensitive) API key for the Claude Platform on AWS gateway. Can also be set via the ANTHROPIC_AWS_API_KEY environment variable. When omitted, SigV4 authentication is resolved via the standard AWS credential chain (environment variables, shared credentials file, IAM role).
- `base_url` (String) Override the Claude Platform on AWS gateway base URL. Can also be set via the ANTHROPIC_AWS_BASE_URL environment variable. Defaults to https://aws-external-anthropic.{region}.api.aws.
- `profile` (String) AWS named profile used to resolve SigV4 credentials via the AWS credential chain.
- `region` (String) AWS region the workspace is bound to. Required for AWS. Can also be set via the AWS_REGION environment variable.
- `workspace_id` (String) Claude Platform on AWS workspace ID (sent as the anthropic-workspace-id header). Required for AWS. Can also be set via the ANTHROPIC_AWS_WORKSPACE_ID environment variable.
