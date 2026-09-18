# AMI resolution.
#
# Ubuntu 24.04 LTS from Canonical's public SSM parameter rather than a literal
# ami-… anywhere. AMI ids are per-region and Canonical republishes them on every
# security refresh, so a hardcoded id is a silent downgrade that gets older every
# week. The parameter is read at plan time; the instance is not replaced unless
# the id it resolves to actually changes.

variable "ami_architecture" {
  description = <<-EOT
    Architecture of the Canonical SSM AMI parameter to resolve: "arm64" or
    "amd64". Must match instance_type — a t4g is arm64, a c7i is amd64. Ignored
    when ami_id is set explicitly.
  EOT
  type        = string
  default     = "amd64"

  validation {
    condition     = contains(["amd64", "arm64"], var.ami_architecture)
    error_message = "ami_architecture must be \"amd64\" or \"arm64\"."
  }
}

data "aws_ssm_parameter" "ubuntu" {
  name = "/aws/service/canonical/ubuntu/server/24.04/stable/current/${var.ami_architecture}/hvm/ebs-gp3/ami-id"
}

locals {
  ami_id = var.ami_id != "" ? var.ami_id : data.aws_ssm_parameter.ubuntu.value
}
