---
name: Two-key provider model — standard vs admin client
description: Provider has two optional clients; which one a resource uses determines which API key it requires
type: project
---

The provider accepts two independent API keys, and at least one must be set:

- `ANTHROPIC_API_KEY` → initialises `ProviderData.Client` (`*anthropic.Client`) — standard SDK, used by most resources and data sources
- `ANTHROPIC_ADMIN_API_KEY` → initialises `ProviderData.AdminClient` (`*admin.Client`) — custom HTTP client for `/v1/organizations/*` endpoints (workspaces, etc.)

Either key alone is sufficient; both can be set simultaneously.

**Which client to use:**
- Default for all new resources/data sources: `pd.Client` (standard SDK)
- Resources managing organization-level objects (workspaces, members): `pd.AdminClient`

**Why:** Changed in session 2026-05-04 to unblock users who only manage workspaces and don't need the standard API key.

**How to apply:** When implementing a new resource, check the API endpoint. If it's under `/v1/organizations/`, use `pd.AdminClient` and `providerrors.RequireAdminResourceClient`. Otherwise use `pd.Client` and `providerrors.RequireResourceAPIClient`.

**Third backend — Claude Platform on AWS (added 2026-06-14):** the `aws { region, workspace_id, api_key, profile, base_url }` provider block selects an AWS-gateway backend for the standard client. `provider.go`'s `newAWSClient` builds the SDK's `*aws.Client` (subpackage `github.com/anthropics/anthropic-sdk-go/aws`) and copies its `Options`/`Completions`/`Messages`/`Models`/`Beta` fields into a `*anthropic.Client`, so `pd.Client` stays `*anthropic.Client` and **standard/beta resources need no AWS-specific code** — keep using `pd.Client` + `RequireResourceAPIClient`. Implication for new resources: anything under `/v1/organizations/*` (Admin) is **not available on AWS** — `pd.AdminClient` is nil there. The provider also has a top-level `base_url` arg (`ANTHROPIC_BASE_URL`) overriding the first-party endpoint for both clients. Conflict/warning checks in `Configure` gate on explicit HCL presence (`isSet`), not env-resolved values. See CLAUDE.md → "Backends: first-party, base_url override, Claude Platform on AWS".
