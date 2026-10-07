# CI deploy path: GitHub Actions -> S3 -> SSM -> bonnie-install.
#
# The design constraint carried over from the hand-ship path (BONNIE.md, and
# the comments in deploy/bonnie-install) is that the privileged action stays
# small enough to read in one sitting. CI does not get a shell on the box. It
# gets exactly two buttons: "stage and activate this sha" and "activate this
# already-present sha". Both are SSM documents whose text lives here, in git,
# reviewed — not strings assembled by a workflow at runtime.
#
# That is why this file does not use AWS-RunShellScript. Permission to run
# AWS-RunShellScript on an instance is permission to be root on it, which would
# make the GitHub OIDC trust policy the only thing standing between a
# compromised Actions run and the box. A pinned document with a hex-validated
# parameter is a far smaller hole.

locals {
  github_repo = "superbuilders/clyde"
}

# ---------------------------------------------------------------------------
# The two things CI is allowed to ask the box to do
# ---------------------------------------------------------------------------

# allowedPattern is load-bearing, not decorative. The parameter is interpolated
# into a shell command, so it is the boundary that stops `sha` from carrying a
# `;`. It is enforced by the SSM service before the command is dispatched,
# i.e. before the box ever sees it. bonnie-stage and bonnie-install each
# re-validate; this is the outermost of three identical checks, and the cheap
# one to get right.
resource "aws_ssm_document" "bonnie_deploy" {
  name            = "${var.name}-deploy"
  document_type   = "Command"
  document_format = "YAML"

  content = yamlencode({
    schemaVersion = "2.2"
    description   = "Stage a CI-built Bonnie release from S3 and activate it."
    parameters = {
      sha = {
        type           = "String"
        description    = "Full 40-character lowercase git commit sha to deploy."
        allowedPattern = "^[0-9a-f]{40}$"
      }
    }
    mainSteps = [
      {
        action = "aws:runShellScript"
        name   = "stageAndActivate"
        inputs = {
          timeoutSeconds = "600"
          runCommand = [
            "set -euo pipefail",
            # Printed before anything moves so the rollback target is always
            # in the command output, even when a later step fails. This is the
            # only reason the deploy path needs to read state at all.
            "echo \"previous: $(readlink /opt/bonnie/current)\"",
            # runuser, not su: no PAM session, no login shell, no chance of a
            # profile script in bonnie-ci's home running as part of a deploy.
            # Download, checksum and unpack all happen as bonnie-ci.
            "runuser -u bonnie-ci -- /usr/local/bin/bonnie-stage '{{ sha }}'",
            # Root's entire contribution: copy three files and flip a symlink.
            "/usr/local/sbin/bonnie-install '{{ sha }}' /srv/bonnie/ci/.ship/'{{ sha }}'",
          ]
        }
      },
    ]
  })
}

resource "aws_ssm_document" "bonnie_rollback" {
  name            = "${var.name}-rollback"
  document_type   = "Command"
  document_format = "YAML"

  content = yamlencode({
    schemaVersion = "2.2"
    description   = "Re-activate a Bonnie version already present under /opt/bonnie/versions."
    parameters = {
      sha = {
        type           = "String"
        description    = "Full 40-character lowercase git commit sha to activate."
        allowedPattern = "^[0-9a-f]{40}$"
      }
    }
    mainSteps = [
      {
        action = "aws:runShellScript"
        name   = "activate"
        inputs = {
          timeoutSeconds = "300"
          # No S3, no unpacking: the bytes are already on disk and were already
          # verified when they were first staged. Rollback must work when the
          # network, the bucket, or CI itself is the thing that is broken.
          runCommand = [
            "set -euo pipefail",
            "/usr/local/sbin/bonnie-install --activate '{{ sha }}'",
          ]
        }
      },
    ]
  })
}

# ---------------------------------------------------------------------------
# The role GitHub Actions assumes
# ---------------------------------------------------------------------------

data "aws_iam_openid_connect_provider" "github" {
  url = "https://token.actions.githubusercontent.com"
}

data "aws_iam_policy_document" "github_actions_assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [data.aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    # Scoped to the environment, not merely to the repo. `repo:org/name:*`
    # would let any branch in the repo — including one a first-time contributor
    # opened a PR from — assume this role. `environment:${var.name}` can only be
    # claimed by a job that declares that environment, and GitHub will not
    # start such a job until the environment's required reviewer approves it.
    # The human sign-off BONNIE.md asks for is therefore enforced by IAM, not
    # just by workflow YAML that a workflow edit could remove.
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${local.github_repo}:environment:${var.name}"]
    }
  }
}

resource "aws_iam_role" "github_actions_deploy" {
  name                 = "${var.name}-ci-deploy"
  description          = "GitHub Actions: publish a Bonnie release and trigger its activation."
  assume_role_policy   = data.aws_iam_policy_document.github_actions_assume.json
  max_session_duration = 3600
}

data "aws_iam_policy_document" "github_actions_deploy" {
  # Write-only, and only under releases/. CI publishes artefacts; it has no
  # business reading back anyone else's, and no business deleting history —
  # every rollback target is an object in this prefix.
  statement {
    sid       = "PublishReleaseArtefacts"
    effect    = "Allow"
    actions   = ["s3:PutObject"]
    resources = ["arn:${data.aws_partition.current.partition}:s3:::${var.release_bucket}/${var.release_prefix}/*"]
  }

  # Refuse to overwrite an existing key. A sha names immutable content; if the
  # object is already there, either it is the same bytes (so the upload is
  # pointless) or someone is rewriting history under a name that is supposed to
  # be a promise. Fail instead.
  statement {
    sid       = "ReleaseArtefactsAreImmutable"
    effect    = "Deny"
    actions   = ["s3:PutObject"]
    resources = ["arn:${data.aws_partition.current.partition}:s3:::${var.release_bucket}/${var.release_prefix}/*"]
    condition {
      test     = "Null"
      variable = "s3:x-amz-copy-source"
      values   = ["true"]
    }
    # NOTE: true immutability wants either a bucket policy with
    # s3:if-none-match or Object Lock in governance mode. The workflow also
    # checks with HeadObject first; this statement is defence in depth and is
    # intentionally the weakest of the three.
  }

  statement {
    sid       = "CheckForExistingArtefact"
    effect    = "Allow"
    actions   = ["s3:ListBucket"]
    resources = ["arn:${data.aws_partition.current.partition}:s3:::${var.release_bucket}"]
    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["${var.release_prefix}/*"]
    }
  }

  # The narrow part: SendCommand is allowed only when BOTH the document and the
  # instance match. SSM evaluates the document arn and the instance arn as
  # separate resources on the same call, so listing both here means a call
  # naming any other document, or any other instance, is denied.
  statement {
    sid     = "RunTheTwoDeployDocuments"
    effect  = "Allow"
    actions = ["ssm:SendCommand"]
    resources = [
      aws_ssm_document.bonnie_deploy.arn,
      aws_ssm_document.bonnie_rollback.arn,
      "arn:${data.aws_partition.current.partition}:ec2:${var.aws_region}:${data.aws_caller_identity.current.account_id}:instance/${aws_instance.web.id}",
    ]
  }

  # Reading back the result is what turns "the API accepted my request" into
  # "the deploy succeeded". Without this the workflow can only ever report the
  # former.
  statement {
    sid    = "ReadCommandResults"
    effect = "Allow"
    actions = [
      "ssm:GetCommandInvocation",
      "ssm:ListCommandInvocations",
      "ssm:ListCommands",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "github_actions_deploy" {
  name   = "${var.name}-ci-deploy"
  role   = aws_iam_role.github_actions_deploy.id
  policy = data.aws_iam_policy_document.github_actions_deploy.json
}

output "github_actions_role_arn" {
  description = "Set as the AWS_DEPLOY_ROLE_ARN repository variable in GitHub."
  value       = aws_iam_role.github_actions_deploy.arn
}

output "bonnie_instance_id" {
  description = "Set as the BONNIE_INSTANCE_ID repository variable in GitHub."
  value       = aws_instance.web.id
}
