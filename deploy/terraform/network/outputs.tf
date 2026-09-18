# The three values the app module takes as inputs, and nothing else.

output "vpc_id" {
  description = "Feed to the app module's vpc_id."
  value       = aws_vpc.main.id
}

output "public_subnet_ids" {
  description = "Feed to the app module's public_subnet_ids (ALB)."
  value       = aws_subnet.public[*].id
}

output "private_subnet_id" {
  description = "Feed to the app module's private_subnet_id (the instance)."
  value       = aws_subnet.private.id
}

output "availability_zone" {
  description = "AZ the instance and its data volume land in."
  value       = aws_subnet.private.availability_zone
}

output "nat_public_ip" {
  description = "Source address the instance appears as to github.com and api.anthropic.com."
  value       = aws_eip.nat.public_ip
}

output "tfvars_snippet" {
  description = "Paste into env/prod.tfvars (gitignored)."
  value = format(
    "vpc_id            = %q\npublic_subnet_ids = %s\nprivate_subnet_id = %q\n",
    aws_vpc.main.id,
    jsonencode(aws_subnet.public[*].id),
    aws_subnet.private.id,
  )
}
