variable "name" {
  description = "Name prefix for every resource, e.g. \"bonnie\"."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,30}$", var.name))
    error_message = "name must be lowercase alphanumeric with hyphens, 2-31 chars."
  }
}

variable "aws_region" {
  description = "Region to create the network in."
  type        = string
}

variable "vpc_cidr" {
  description = <<-EOT
    CIDR for the dedicated Bonnie VPC. Dedicated on purpose: the instance is a
    shared multi-tenant computer running agent-authored code, so it does not
    belong in a VPC alongside unrelated workloads — an SSRF or a compromised
    agent would otherwise start with network reachability to them
    (docs/access-model.md §3, PLAN §M6).
  EOT
  type        = string
  default     = "10.60.0.0/16"
}

variable "public_subnet_cidrs" {
  description = "Two /24s in different AZs. ALB and NAT only; nothing of ours runs here."
  type        = list(string)
  default     = ["10.60.0.0/24", "10.60.1.0/24"]

  validation {
    condition     = length(var.public_subnet_cidrs) == 2
    error_message = "Exactly two public subnets: an ALB needs two AZs, and a third would only cost more."
  }
}

variable "private_subnet_cidr" {
  description = "The instance's subnet. No public IP, egress via NAT (DESIGN §10)."
  type        = string
  default     = "10.60.10.0/24"
}

variable "tags" {
  description = "Tags applied to every resource."
  type        = map(string)
  default     = {}
}
