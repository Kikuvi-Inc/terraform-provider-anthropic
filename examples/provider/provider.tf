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
