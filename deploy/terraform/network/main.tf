# deploy/terraform/network/main.tf — the dedicated network the app module expects.
#
#   two public subnets (two AZs)  -> ALB, NAT gateway
#   one private subnet            -> the instance: no public IP, no ssh
#   IGW for the public side, one NAT gateway for private egress
#
# Why a dedicated VPC rather than an existing one: the instance is a shared
# computer that runs agent-authored code as real humans. Its blast radius must be
# a network it is alone in, so that the only things it can reach are the internet
# (github.com, api.anthropic.com) and the AWS APIs its instance profile allows —
# not whatever else happens to live in a shared VPC (access-model §3, PLAN §M6).
#
# One NAT gateway, not one per AZ. There is one instance in one AZ (DESIGN §10),
# so a second NAT would buy availability for a subnet with nothing in it and cost
# another ~$33/month.

provider "aws" {
  region = var.aws_region
}

locals {
  tags = merge(var.tags, {
    Name      = "${var.name}-network"
    ManagedBy = "terraform"
    Component = "bonnie"
    Project   = "bonnie"
  })
}

# Only AZs that can actually take an instance today; hardcoding us-east-1a/b is
# how you discover a capacity-constrained AZ at apply time.
data "aws_availability_zones" "available" {
  state = "available"

  filter {
    name   = "opt-in-status"
    values = ["opt-in-not-required"]
  }
}

resource "aws_vpc" "main" {
  cidr_block = var.vpc_cidr
  # DNS hostnames and support are both required for the SSM agent to resolve the
  # regional endpoints it dials out to, which is the only admin path in (DESIGN §10).
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = merge(local.tags, { Name = "${var.name}-vpc" })
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags   = merge(local.tags, { Name = "${var.name}-igw" })
}

# ---------------------------------------------------------------------------
# Public subnets — ALB and NAT only
# ---------------------------------------------------------------------------

resource "aws_subnet" "public" {
  count = length(var.public_subnet_cidrs)

  vpc_id            = aws_vpc.main.id
  cidr_block        = var.public_subnet_cidrs[count.index]
  availability_zone = data.aws_availability_zones.available.names[count.index]

  # The ALB needs public IPs on its own ENIs, which it assigns itself. Nothing of
  # ours is ever launched here, so auto-assign stays off: a subnet that hands out
  # public IPs by default is how an instance ends up reachable by accident.
  map_public_ip_on_launch = false

  tags = merge(local.tags, {
    Name = "${var.name}-public-${data.aws_availability_zones.available.names[count.index]}"
    Tier = "public"
  })
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id
  tags   = merge(local.tags, { Name = "${var.name}-public" })
}

resource "aws_route" "public_default" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.main.id
}

resource "aws_route_table_association" "public" {
  count = length(aws_subnet.public)

  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# ---------------------------------------------------------------------------
# Private subnet — the instance
# ---------------------------------------------------------------------------

resource "aws_subnet" "private" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = var.private_subnet_cidr
  availability_zone = data.aws_availability_zones.available.names[0]

  map_public_ip_on_launch = false

  tags = merge(local.tags, {
    Name = "${var.name}-private-${data.aws_availability_zones.available.names[0]}"
    Tier = "private"
  })
}

resource "aws_eip" "nat" {
  domain = "vpc"
  tags   = merge(local.tags, { Name = "${var.name}-nat" })

  depends_on = [aws_internet_gateway.main]
}

# NAT rather than an egress-only IGW or a pile of VPC endpoints: the box has to
# reach github.com and api.anthropic.com over IPv4, which endpoints cannot do.
resource "aws_nat_gateway" "main" {
  allocation_id = aws_eip.nat.id
  # In the same AZ as the private subnet, or every byte of egress crosses an AZ
  # boundary and gets charged for it.
  subnet_id = aws_subnet.public[0].id

  tags = merge(local.tags, { Name = "${var.name}-nat" })

  depends_on = [aws_internet_gateway.main]
}

resource "aws_route_table" "private" {
  vpc_id = aws_vpc.main.id
  tags   = merge(local.tags, { Name = "${var.name}-private" })
}

resource "aws_route" "private_default" {
  route_table_id         = aws_route_table.private.id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.main.id
}

resource "aws_route_table_association" "private" {
  subnet_id      = aws_subnet.private.id
  route_table_id = aws_route_table.private.id
}

# S3 gateway endpoint. Free, and the release tarball is pulled from S3 on every
# boot and every deploy, so this keeps the largest transfer off the metered NAT.
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.main.id
  service_name      = "com.amazonaws.${var.aws_region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.private.id]

  tags = merge(local.tags, { Name = "${var.name}-s3" })
}
