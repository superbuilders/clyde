# Every variable here is either required (no default) or has a default that is
# generic and safe for any installation. There are no account ids, hostnames,
# secret names, AMI ids or role names with defaults — those are the facts that
# differ per install, they belong in a .tfvars that is not committed, and the
# denylist CI job (scripts/denylist.sh) fails the build if one leaks in
# (repo-split §3).

# ---------------------------------------------------------------------------
# Naming and placement
# ---------------------------------------------------------------------------

variable "name" {
  description = "Name prefix for every resource, e.g. \"bonnie\" or \"bonnie-dev\"."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,30}$", var.name))
    error_message = "name must be lowercase alphanumeric with hyphens, 2-31 chars."
  }
}

variable "aws_region" {
  description = "Region to deploy into. One region, one instance (DESIGN §10)."
  type        = string
}

variable "vpc_id" {
  description = "Existing VPC id. This module does not create a VPC."
  type        = string
}

variable "public_subnet_ids" {
  description = "Subnets for the ALB. At least two AZs, internet-facing."
  type        = list(string)

  validation {
    condition     = length(var.public_subnet_ids) >= 2
    error_message = "An ALB requires subnets in at least two availability zones."
  }
}

variable "private_subnet_id" {
  description = <<-EOT
    Subnet for the instance. Must be private: the instance gets no public IP and
    no ssh ingress, and admin access is SSM Session Manager only (DESIGN §10).
    Needs a NAT gateway or VPC endpoints for SSM, S3 and Secrets Manager.
  EOT
  type        = string
}

# ---------------------------------------------------------------------------
# Instance
# ---------------------------------------------------------------------------

variable "instance_type" {
  description = <<-EOT
    One instance, not a fleet. The workload is long-lived stateful processes on a
    shared filesystem, so vertical is the axis that matters (DESIGN §10).
  EOT
  type        = string
  default     = "c7i.2xlarge"
}

variable "ami_id" {
  description = <<-EOT
    Ubuntu 24.04 LTS AMI id, as an override. Leave empty — the default — and the
    module resolves it from Canonical's public SSM parameter for
    var.ami_architecture (see ami.tf). AMI ids are per-region and Canonical
    republishes them on every security refresh, so a hardcoded one is a silent
    downgrade; this variable exists only to pin an image deliberately.
  EOT
  type        = string
  default     = ""

  validation {
    condition     = var.ami_id == "" || can(regex("^ami-[0-9a-f]{8,}$", var.ami_id))
    error_message = "ami_id must be empty (resolve from SSM) or a well-formed ami- id."
  }
}

variable "root_volume_size_gb" {
  description = "Root volume size. Holds the OS and /opt/bonnie/versions only."
  type        = number
  default     = 50
}

variable "data_volume_size_gb" {
  description = "Size of the separate /srv/bonnie volume (DESIGN §10)."
  type        = number
  default     = 200
}

# Deliberately no data_volume_device variable. Nothing outside the instance can
# know which nvme node the kernel will hand a volume, and this AMI has no udev
# rule that recreates the attachment name, so the mount is keyed on the volume's
# NVMe serial instead — local.data_volume_by_id in main.tf. A variable here would
# only be a guess with a config knob on it.

variable "data_volume_attachment_name" {
  description = "Block device name used for the EBS attachment API call."
  type        = string
  default     = "/dev/sdf"
}

variable "kms_key_arn" {
  description = <<-EOT
    CMK for EBS and snapshot encryption. Empty string uses the AWS-managed EBS
    key. Volumes are encrypted either way; this only chooses the key.
  EOT
  type        = string
  default     = ""
}

# ---------------------------------------------------------------------------
# DNS and TLS
# ---------------------------------------------------------------------------

variable "domain_name" {
  description = "Fully-qualified hostname to serve on. No default — repo-split §3."
  type        = string
}

variable "route53_zone_id" {
  description = "Hosted zone that contains domain_name."
  type        = string
}

# ---------------------------------------------------------------------------
# Release artefacts
# ---------------------------------------------------------------------------

variable "release_bucket" {
  description = "S3 bucket holding release tarballs (DESIGN §13.4)."
  type        = string
}

variable "release_prefix" {
  description = "Key prefix within release_bucket."
  type        = string
  default     = "releases"
}

variable "release_version" {
  description = <<-EOT
    Git SHA of the release to install at first boot. Later deploys move the
    /opt/bonnie/current symlink and do NOT re-run Terraform (DESIGN §13.1) — this
    value only decides where a freshly built instance starts.
  EOT
  type        = string
}

# ---------------------------------------------------------------------------
# Secrets
# ---------------------------------------------------------------------------

variable "secret_prefix" {
  description = <<-EOT
    Secrets Manager name prefix. The instance profile is scoped to
    GetSecretValue on "<secret_prefix>/*" and nothing else, so this is a
    security-relevant value: widening it widens what a compromised instance can
    read. Trailing slash is added automatically.
  EOT
  type        = string
  default     = "bonnie"

  validation {
    condition     = can(regex("^[A-Za-z0-9/_+=.@-]{1,256}$", var.secret_prefix)) && !endswith(var.secret_prefix, "/")
    error_message = "secret_prefix must be a valid secret name fragment without a trailing slash."
  }
}

variable "web_secret_name" {
  description = <<-EOT
    Secrets Manager secret holding the web process's JSON secret bundle (OIDC
    client secret, session signing key). Must sit under secret_prefix. No default,
    and the .tfvars.example uses a placeholder.
  EOT
  type        = string
}

variable "agent_secret_name" {
  description = "Secret holding the agent model credential (gateway key, URL, model id)."
  type        = string
}

variable "github_secret_name" {
  description = "Secret holding the agent GitHub credential. Optional: if it does not exist the box still boots, but private git remotes will not work."
  type        = string
  default     = ""
}


# ---------------------------------------------------------------------------
# Backups and alarms
# ---------------------------------------------------------------------------

variable "snapshot_retain_count" {
  description = "Nightly DLM snapshots of the data volume to retain (DESIGN §10)."
  type        = number
  default     = 14
}

variable "snapshot_time_utc" {
  description = "Nightly snapshot start time, HH:MM UTC."
  type        = string
  default     = "07:00"

  validation {
    condition     = can(regex("^([01][0-9]|2[0-3]):[0-5][0-9]$", var.snapshot_time_utc))
    error_message = "snapshot_time_utc must be HH:MM in 24h UTC."
  }
}

variable "alarm_sns_topic_arn" {
  description = <<-EOT
    SNS topic for alarm notifications. Empty string creates the alarms with no
    action, which is still useful (they show in the console) but will not page
    anyone.
  EOT
  type        = string
  default     = ""
}

variable "disk_used_alarm_percent" {
  description = "Alarm when /srv/bonnie is this full. Sessions are markdown, so a full disk means something is wrong, not that we grew."
  type        = number
  default     = 80
}

variable "memory_used_alarm_percent" {
  description = "Alarm when instance memory use exceeds this. Per-user MemoryMax bounds each agent; this catches the aggregate (access-model §4)."
  type        = number
  default     = 85
}

variable "log_retention_days" {
  description = "CloudWatch log group retention for journald output."
  type        = number
  default     = 90
}

variable "tags" {
  description = "Tags applied to every resource that supports them."
  type        = map(string)
  default     = {}
}

# ---------------------------------------------------------------------------
# Auth (M1). Non-secret OIDC configuration; the client secret and the session
# signing key arrive from Secrets Manager at boot, never from Terraform state.
# ---------------------------------------------------------------------------

variable "oidc_issuer" {
  description = "OIDC issuer URL (the Cognito user pool)."
  type        = string
}

variable "oidc_client_id" {
  description = "OIDC app client id. Not a secret; the secret is in Secrets Manager."
  type        = string
}

variable "allowed_emails" {
  description = <<-EOT
    Comma-separated allowlist of bare domains and/or exact addresses. This is the
    security boundary of the whole install: anyone the issuer will authenticate
    AND who matches this list gets in. M2 restricts it to AJ plus the e2e user.
  EOT
  type        = string
}
