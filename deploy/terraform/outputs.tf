# Outputs are the handful of facts the release pipeline needs. Deliberately narrow:
# anything printed here ends up in CI logs, so no ARNs of secrets and no account
# identifiers (repo-split §3).

output "instance_id" {
  description = "Target for SSM Run Command during a deploy (DESIGN §13.4)."
  value       = aws_instance.web.id
}

output "url" {
  description = "Where Bonnie is served."
  value       = "https://${var.domain_name}"
}

output "alb_dns_name" {
  description = "ALB hostname, for a Route53 record managed outside this module."
  value       = aws_lb.web.dns_name
}

output "target_group_arn_suffix" {
  description = "For reading HealthyHostCount during a deploy."
  value       = aws_lb_target_group.web.arn_suffix
}

output "log_group_name" {
  description = "CloudWatch log group carrying journald and the audit log."
  value       = aws_cloudwatch_log_group.journal.name
}

output "data_volume_id" {
  description = "The volume every session lives on. Snapshot before any risky maintenance."
  value       = aws_ebs_volume.data.id
}

output "instance_role_name" {
  description = "Name (not ARN) of the instance role, for attaching install-specific policies from another module."
  value       = aws_iam_role.instance.name
}
