terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.40.0"
    }
  }

  # Same reasoning as the app module: state carries subnet and VPC ids, which are
  # per-install facts (repo-split §3). Configure with a backend file that stays
  # out of git:
  #
  #   terraform init -backend-config=../env/prod.backend.hcl \
  #     -backend-config='key=bonnie/network.tfstate'
  #
  # An empty block: every setting (bucket, key, region) arrives from the backend
  # config file above, so no per-account fact is written down here.
  backend "s3" {}
}
