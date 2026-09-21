# deploy/terraform/main.tf — the reference deployment from DESIGN §10.
#
#   Route53 -> ALB (:443, ACM cert) -> EC2 instance (:8080, ALB-only SG)
#   /srv/bonnie on a separate encrypted EBS volume, nightly DLM snapshots
#   SSM Session Manager for admin access: no key pair, no public IP, no ssh
#
# One instance on purpose. The product is a persistent computer with long-lived
# stateful processes on a shared filesystem; ECS/Fargate would need EFS and a
# pooled storage mode and would still be a worse fit (DESIGN §10).

provider "aws" {
  region = var.aws_region
}

data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}

# Only for the data volume's availability zone — see aws_ebs_volume.data.
data "aws_subnet" "private" {
  id = var.private_subnet_id
}

locals {
  tags = merge(var.tags, {
    Name      = var.name
    ManagedBy = "terraform"
    Component = "bonnie"
  })

  # Constrains the instance profile to this install's secrets only. Built from a
  # variable so no account id or secret name appears in the repo (repo-split §3).
  secret_arn_glob = format(
    "arn:%s:secretsmanager:%s:%s:secret:%s/*",
    data.aws_partition.current.partition,
    var.aws_region,
    data.aws_caller_identity.current.account_id,
    var.secret_prefix,
  )

  log_group_name = "/bonnie/${var.name}/journal"

  # base64 rather than the raw TOML: cloud-init user-data is YAML, and a multi-line
  # value with its own quoting would have to be re-indented correctly by
  # templatefile(). One opaque line cannot be broken by a comment or a string in
  # the config. Validated by `bonnie doctor` on the box, not here.

  # How the instance finds its data volume. EBS publishes the volume id (minus the
  # dash) as the NVMe serial, and Ubuntu's stock udev rules turn that into a
  # /dev/disk/by-id symlink, so this path names one specific volume for as long as
  # it exists. The attachment name (/dev/sdf) is not usable: on Nitro it is not a
  # device node, and this AMI has no ebsnvme-id udev rule to symlink it.
  data_volume_by_id = "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_${replace(aws_ebs_volume.data.id, "-", "")}"
}

# ---------------------------------------------------------------------------
# TLS certificate
# ---------------------------------------------------------------------------

resource "aws_acm_certificate" "web" {
  domain_name       = var.domain_name
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }

  tags = local.tags
}

resource "aws_route53_record" "cert_validation" {
  for_each = {
    for dvo in aws_acm_certificate.web.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  zone_id         = var.route53_zone_id
  name            = each.value.name
  type            = each.value.type
  records         = [each.value.record]
  ttl             = 60
  allow_overwrite = true
}

resource "aws_acm_certificate_validation" "web" {
  certificate_arn         = aws_acm_certificate.web.arn
  validation_record_fqdns = [for r in aws_route53_record.cert_validation : r.fqdn]
}

# ---------------------------------------------------------------------------
# Security groups
#
# Two groups, and the instance group's only ingress rule references the ALB group
# by id rather than by CIDR. That is what makes "the instance is unreachable
# except through the ALB" a property of the infrastructure rather than of a subnet
# layout someone may later change.
# ---------------------------------------------------------------------------

resource "aws_security_group" "alb" {
  name        = "${var.name}-alb"
  description = "Bonnie ALB: 443 in from the internet"
  vpc_id      = var.vpc_id
  tags        = local.tags
}

resource "aws_vpc_security_group_ingress_rule" "alb_https" {
  security_group_id = aws_security_group.alb.id
  # The § in "DESIGN §4" is not in the character set EC2 accepts for rule
  # descriptions, hence the spelled-out reference.
  description = "HTTPS from anywhere; auth happens at the app (DESIGN section 4)"
  cidr_ipv4   = "0.0.0.0/0"
  from_port   = 443
  to_port     = 443
  ip_protocol = "tcp"
}

# Port 80 reaches the ALB only so it can be told to go away: the listener's sole
# action is a 301 to https. Without this rule the :80 listener exists and nothing
# can connect to it, so a plain http:// link times out instead of upgrading —
# which is worse than not offering http at all.
resource "aws_vpc_security_group_ingress_rule" "alb_http" {
  security_group_id = aws_security_group.alb.id
  description       = "HTTP from anywhere; the listener only 301s to HTTPS"
  cidr_ipv4         = "0.0.0.0/0"
  from_port         = 80
  to_port           = 80
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_egress_rule" "alb_to_instance" {
  security_group_id            = aws_security_group.alb.id
  description                  = "Only to the Bonnie instance on 8080"
  referenced_security_group_id = aws_security_group.instance.id
  from_port                    = 8080
  to_port                      = 8080
  ip_protocol                  = "tcp"
}

resource "aws_security_group" "instance" {
  name        = "${var.name}-instance"
  description = "Bonnie instance: 8080 in from the ALB only, no ssh"
  vpc_id      = var.vpc_id
  tags        = local.tags
}

resource "aws_vpc_security_group_ingress_rule" "instance_from_alb" {
  security_group_id            = aws_security_group.instance.id
  description                  = "bonnie-web.socket, ALB only"
  referenced_security_group_id = aws_security_group.alb.id
  from_port                    = 8080
  to_port                      = 8080
  ip_protocol                  = "tcp"
}

# No port 22 rule anywhere in this file, and that is deliberate: admin access is
# SSM Session Manager, which is an outbound connection from the agent. Adding an
# ssh ingress rule here would also require a key pair, a bastion and a public IP,
# all three of which DESIGN §10 explicitly rejects.

resource "aws_vpc_security_group_egress_rule" "instance_all" {
  security_group_id = aws_security_group.instance.id
  description       = "git clone, package updates, SSM, S3, Secrets Manager, Anthropic API"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

# ---------------------------------------------------------------------------
# Instance profile
#
# Three things and no more: SSM for admin shell, read-only S3 on the release
# prefix, and GetSecretValue scoped to this install's secret prefix. Notably NOT
# ssm:SendCommand — a deploy is issued from CI to the instance, so the instance
# never needs to command itself or anything else.
# ---------------------------------------------------------------------------

data "aws_iam_policy_document" "assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "instance" {
  name               = "${var.name}-instance"
  assume_role_policy = data.aws_iam_policy_document.assume.json
  tags               = local.tags
}

resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.instance.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_role_policy_attachment" "cloudwatch" {
  role       = aws_iam_role.instance.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/CloudWatchAgentServerPolicy"
}

data "aws_iam_policy_document" "instance" {
  # Secrets. The resource is a glob under one prefix, the action is exactly one,
  # and there is no wildcard on either. A compromised instance can read this
  # install's secrets — which it must, to work — and nothing else in the account.
  statement {
    sid       = "ReadBonnieSecretsOnly"
    actions   = ["secretsmanager:GetSecretValue"]
    resources = [local.secret_arn_glob]
  }

  statement {
    sid       = "ReadReleaseArtefacts"
    actions   = ["s3:GetObject"]
    resources = ["arn:${data.aws_partition.current.partition}:s3:::${var.release_bucket}/${var.release_prefix}/*"]
  }

  statement {
    sid       = "ListReleaseBucketPrefix"
    actions   = ["s3:ListBucket"]
    resources = ["arn:${data.aws_partition.current.partition}:s3:::${var.release_bucket}"]
    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["${var.release_prefix}/*"]
    }
  }
}

resource "aws_iam_role_policy" "instance" {
  name   = "${var.name}-instance"
  role   = aws_iam_role.instance.id
  policy = data.aws_iam_policy_document.instance.json
}

resource "aws_iam_instance_profile" "instance" {
  name = "${var.name}-instance"
  role = aws_iam_role.instance.name
  tags = local.tags
}

# ---------------------------------------------------------------------------
# CloudWatch agent config, rendered here so cloud-init carries it
# ---------------------------------------------------------------------------

resource "aws_cloudwatch_log_group" "journal" {
  name              = local.log_group_name
  retention_in_days = var.log_retention_days
  tags              = local.tags
}

locals {
  cloudwatch_agent_config = jsonencode({
    agent = {
      metrics_collection_interval = 60
      run_as_user                 = "root"
    }
    metrics = {
      namespace = "Bonnie/${var.name}"
      append_dimensions = {
        InstanceId = "$${aws:InstanceId}"
      }
      metrics_collected = {
        # Only /srv/bonnie matters for capacity: it is where sessions, repos and
        # workspaces live. The root volume holds releases and is GC'd by
        # deploy/release/gc-versions.sh.
        disk = {
          resources                = ["/", "/srv/bonnie"]
          measurement              = ["used_percent", "inodes_free"]
          ignore_file_system_types = ["sysfs", "devtmpfs", "tmpfs"]
        }
        mem = {
          measurement = ["mem_used_percent", "mem_available"]
        }
        # One process per agent, so process count is a useful proxy for "how many
        # people are actually using this".
        procstat = [
          { exe = "bonnie", measurement = ["cpu_usage", "memory_rss"] },
          { exe = "clyde", measurement = ["cpu_usage", "memory_rss"] },
          { exe = "tmux", measurement = ["cpu_usage", "memory_rss"] },
        ]
      }
    }
    logs = {
      logs_collected = {
        files = {
          collect_list = [
            {
              # The audit log is the one piece of security state the kernel does
              # not enforce for us, so it gets shipped off the box where an agent
              # cannot reach it at all (access-model §6).
              file_path       = "/srv/bonnie/var/audit/*.log"
              log_group_name  = local.log_group_name
              log_stream_name = "{instance_id}/audit"
              timezone        = "UTC"
            }
          ]
        }
        # journald, not a file tail of syslog: every unit here logs to the journal
        # and nothing writes /var/log/syslog for us. Two entries into one group —
        # the web unit gets its own stream because it is the one anybody reads
        # during an incident, and everything else lands in a catch-all so a
        # failure in provisioning or an agent is not invisible.
        journald = {
          collect_list = [
            {
              log_group_name  = local.log_group_name
              log_stream_name = "{instance_id}/bonnie-web"
              units           = ["bonnie-web.service"]
            },
            {
              log_group_name  = local.log_group_name
              log_stream_name = "{instance_id}/journal"
            },
          ]
        }
      }
    }
  })
}

# ---------------------------------------------------------------------------
# Instance and data volume
# ---------------------------------------------------------------------------

resource "aws_instance" "web" {
  ami                    = local.ami_id
  instance_type          = var.instance_type
  subnet_id              = var.private_subnet_id
  vpc_security_group_ids = [aws_security_group.instance.id]
  iam_instance_profile   = aws_iam_instance_profile.instance.name

  # No key pair and no public IP. Both are load-bearing, not cosmetic: with a key
  # pair there is a credential to lose, and with a public IP there is a surface to
  # scan (DESIGN §10).
  key_name                    = null
  associate_public_ip_address = false

  # IMDSv2 only: SSRF in the web process must not be able to read the instance
  # profile credentials with a plain GET.
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  root_block_device {
    volume_type = "gp3"
    volume_size = var.root_volume_size_gb
    encrypted   = true
    kms_key_id  = var.kms_key_arn != "" ? var.kms_key_arn : null
    tags        = local.tags
  }

  # gzip + base64, not the raw text. EC2 caps user-data at 16 KiB and this
  # cloud-config is mostly comments explaining security-relevant choices, which are
  # the last thing that should be deleted to fit a size limit. cloud-init
  # transparently gunzips user-data, so this costs nothing at boot.
  user_data_base64 = base64gzip(templatefile("${path.module}/../cloud-init.yaml", {
    aws_region              = var.aws_region
    data_volume_by_id       = local.data_volume_by_id
    release_bucket          = var.release_bucket
    release_prefix          = var.release_prefix
    release_version         = var.release_version
    web_secret_name         = var.web_secret_name
    agent_secret_name       = var.agent_secret_name
    github_secret_name      = var.github_secret_name
    domain_name             = var.domain_name
    oidc_issuer             = var.oidc_issuer
    oidc_client_id          = var.oidc_client_id
    allowed_emails          = var.allowed_emails
    cloudwatch_agent_config = local.cloudwatch_agent_config
    systemd_unit            = indent(6, file("${path.module}/../systemd/bonnie-web.service"))
  }))

  # Replacing the instance must not replace the data. The data volume is a
  # separate resource with prevent_destroy, and it reattaches.
  user_data_replace_on_change = false

  tags = local.tags
}

# The AZ comes from the private subnet, not from aws_instance.web.availability_zone.
# They are always the same value, but reading it off the instance would make the
# volume depend on the instance, and the instance's user-data has to embed this
# volume's id (local.data_volume_by_id) — that is a dependency cycle. The subnet is
# what actually determines the AZ anyway.
resource "aws_ebs_volume" "data" {
  availability_zone = data.aws_subnet.private.availability_zone
  size              = var.data_volume_size_gb
  type              = "gp3"
  encrypted         = true
  kms_key_id        = var.kms_key_arn != "" ? var.kms_key_arn : null

  # Every session anyone has ever had is on this volume, and it is not
  # reconstructible from anything else. Removing it from the config should require
  # removing this block first, deliberately.
  lifecycle {
    prevent_destroy = true
  }

  tags = merge(local.tags, {
    Name           = "${var.name}-data"
    SnapshotPolicy = var.name
  })
}

resource "aws_volume_attachment" "data" {
  device_name = var.data_volume_attachment_name
  volume_id   = aws_ebs_volume.data.id
  instance_id = aws_instance.web.id
}

# ---------------------------------------------------------------------------
# Nightly snapshots (DLM)
#
# Sessions are markdown; they compress to nothing and restore by copying, so a
# nightly crash-consistent snapshot is a genuinely adequate backup (DESIGN §10).
# ---------------------------------------------------------------------------

data "aws_iam_policy_document" "dlm_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["dlm.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "dlm" {
  name               = "${var.name}-dlm"
  assume_role_policy = data.aws_iam_policy_document.dlm_assume.json
  tags               = local.tags
}

resource "aws_iam_role_policy_attachment" "dlm" {
  role       = aws_iam_role.dlm.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AWSDataLifecycleManagerServiceRole"
}

resource "aws_dlm_lifecycle_policy" "data" {
  # DLM validates the description against [0-9A-Za-z _-]+, so no slashes: this is
  # the /srv/bonnie volume's policy, spelled the only way the API accepts.
  description        = "${var.name} nightly srv-bonnie data volume snapshots"
  execution_role_arn = aws_iam_role.dlm.arn
  state              = "ENABLED"

  policy_details {
    resource_types = ["VOLUME"]

    # Tag-selected rather than id-selected so replacing the volume (a restore)
    # does not silently stop the backups.
    target_tags = {
      SnapshotPolicy = var.name
    }

    schedule {
      name = "nightly"

      create_rule {
        interval      = 24
        interval_unit = "HOURS"
        times         = [var.snapshot_time_utc]
      }

      retain_rule {
        count = var.snapshot_retain_count
      }

      copy_tags = true

      tags_to_add = {
        SnapshotCreator = "dlm"
        Component       = "bonnie"
      }
    }
  }

  tags = local.tags
}

# ---------------------------------------------------------------------------
# ALB
# ---------------------------------------------------------------------------

resource "aws_lb" "web" {
  name               = "${var.name}-alb"
  internal           = false
  load_balancer_type = "application"
  subnets            = var.public_subnet_ids
  security_groups    = [aws_security_group.alb.id]

  # Long: a session view holds an SSE/websocket pane stream open for as long as
  # someone is watching, and a 60s idle timeout would cut it (DESIGN §13.3 blip).
  idle_timeout = 3600

  drop_invalid_header_fields = true
  enable_http2               = true

  tags = local.tags
}

resource "aws_lb_target_group" "web" {
  name        = "${var.name}-tg"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "instance"

  # Health check must be cheap and must not require auth — activate.sh polls the
  # same endpoint before and after flipping the symlink (DESIGN §13.4).
  health_check {
    path                = "/healthz"
    matcher             = "200"
    interval            = 10
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  # Socket activation means an upgrade queues connections rather than refusing
  # them, so a short deregistration delay is safe.
  deregistration_delay = 15

  tags = local.tags
}

resource "aws_lb_target_group_attachment" "web" {
  target_group_arn = aws_lb_target_group.web.arn
  target_id        = aws_instance.web.id
  port             = 8080
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.web.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate_validation.web.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.web.arn
  }
}

resource "aws_lb_listener" "http_redirect" {
  load_balancer_arn = aws_lb.web.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_route53_record" "web" {
  zone_id = var.route53_zone_id
  name    = var.domain_name
  type    = "A"

  alias {
    name                   = aws_lb.web.dns_name
    zone_id                = aws_lb.web.zone_id
    evaluate_target_health = true
  }
}

# ---------------------------------------------------------------------------
# Alarms
#
# Three that actually mean something on a single-instance deployment: the box is
# gone, the disk is filling, memory is exhausted. No CPU alarm — a busy agent
# pegging a core is the system working, and CPUQuota= already bounds it per user
# (access-model §4).
# ---------------------------------------------------------------------------

locals {
  alarm_actions = var.alarm_sns_topic_arn != "" ? [var.alarm_sns_topic_arn] : []
}

resource "aws_cloudwatch_metric_alarm" "instance_health" {
  alarm_name          = "${var.name}-instance-unhealthy"
  alarm_description   = "EC2 status check failed; the instance is the service (DESIGN §10)."
  namespace           = "AWS/EC2"
  metric_name         = "StatusCheckFailed"
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 2
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "breaching"

  dimensions = {
    InstanceId = aws_instance.web.id
  }

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
  tags          = local.tags
}

resource "aws_cloudwatch_metric_alarm" "target_health" {
  alarm_name          = "${var.name}-no-healthy-targets"
  alarm_description   = "ALB has no healthy target: bonnie-web is down or failing /healthz."
  namespace           = "AWS/ApplicationELB"
  metric_name         = "HealthyHostCount"
  statistic           = "Minimum"
  period              = 60
  evaluation_periods  = 2
  threshold           = 1
  comparison_operator = "LessThanThreshold"
  treat_missing_data  = "breaching"

  dimensions = {
    LoadBalancer = aws_lb.web.arn_suffix
    TargetGroup  = aws_lb_target_group.web.arn_suffix
  }

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
  tags          = local.tags
}

resource "aws_cloudwatch_metric_alarm" "disk" {
  alarm_name          = "${var.name}-data-disk-full"
  alarm_description   = "/srv/bonnie above ${var.disk_used_alarm_percent}% used. Sessions are markdown, so this usually means a runaway checkout, not growth."
  namespace           = "Bonnie/${var.name}"
  metric_name         = "disk_used_percent"
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 2
  threshold           = var.disk_used_alarm_percent
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "missing"

  dimensions = {
    InstanceId = aws_instance.web.id
    path       = "/srv/bonnie"
  }

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
  tags          = local.tags
}

resource "aws_cloudwatch_metric_alarm" "memory" {
  alarm_name          = "${var.name}-memory-pressure"
  alarm_description   = "Instance memory above ${var.memory_used_alarm_percent}%. Per-agent MemoryMax bounds one user; this catches the aggregate."
  namespace           = "Bonnie/${var.name}"
  metric_name         = "mem_used_percent"
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  threshold           = var.memory_used_alarm_percent
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "missing"

  dimensions = {
    InstanceId = aws_instance.web.id
  }

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
  tags          = local.tags
}
