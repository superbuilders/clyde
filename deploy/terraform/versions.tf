terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.40.0"
    }
  }

  # Backend is deliberately unconfigured. State for this module contains the
  # instance id, the DNS name and the ARNs of the secrets it reads, i.e. exactly
  # the environment-specific facts that repo-split §3 says must not live in the
  # public repo. Configure it with `-backend-config` or a backend file that stays
  # out of git:
  #
  #   terraform init -backend-config=env/prod.backend.hcl
  #
  # An empty block: every setting (bucket, key, region) arrives from the backend
  # config file, so no per-account fact is written down here. The block must be
  # present, though — with it absent Terraform silently uses local state and
  # ignores -backend-config entirely.
  backend "s3" {}
}
