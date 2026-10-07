# Deploying Bonnie from GitHub Actions

This describes the CI ship path: a commit on GitHub becomes the running
`/opt/bonnie/current` without anyone opening a shell on the box.

It does not replace `scripts/bonnie-ship.sh`. Both paths converge on the same
privileged action, `bonnie-install`, and both leave every version on disk so
rollback is a symlink flip. Use whichever is available; the box path is faster,
the CI path works when the box is not serving and produces an artefact someone
other than its builder can verify.

## Why this shape

The existing hand-ship design already made the only decision that matters:
**root copies three files and flips a symlink, and does nothing else.** Root
does not build, does not download, does not unpack, and never executes a script
out of a build directory. That is what keeps the trusted action short enough to
read in one sitting, and it is the property every choice below is protecting.

Adding CI threatens that property in a specific way. The natural implementation
is to give the Actions role `ssm:SendCommand` on `AWS-RunShellScript`, and that
single grant is equivalent to root on the box. The whole careful split collapses
into "whoever can trigger a workflow is root", and the only remaining control is
the OIDC trust policy.

So CI does not get a shell. It gets two buttons.

## The pipeline

```
 GitHub Actions                        S3                      the box
 ─────────────────────────────────────────────────────────────────────────────
 build job
   (no credentials)
   go vet / go test
   go build  ──────────▶  <sha>.tar.gz
                          <sha>.tar.gz.sha256
         │
         ▼
 deploy job
   environment: bonnie-dev      ← human approval happens here
   OIDC ▶ bonnie-ci-deploy
   s3:PutObject ────────────▶ releases/<sha>.tar.gz
   ssm:SendCommand
     document: bonnie-deploy ──────────────────────────▶ runuser -u bonnie-ci
                                                           bonnie-stage <sha>
                                                             fetch, verify
                                                             sha256, unpack,
                                                             assert ELF
                                                        ────────────────────
                                                        bonnie-install <sha> …
                                                           cp ×3, symlink,
                                                           provision, restart,
                                                           healthz
```

By the time `bonnie-install` runs, the staged directory is three files on disk,
byte-identical in kind to what a human build produces. `bonnie-install` is
unchanged and cannot tell the two paths apart — which is the point. It is the
piece we most want to keep stable.

## The four controls

**1. The build job holds no credentials.** It is the only job that executes code
from the ref being deployed — `go build` runs the repo's own toolchain
directive and any generators. That is the step most exposed to a malicious
commit, so it has nothing to steal. The deploy job holds the credentials and
checks nothing out.

**2. CI can run exactly two SSM documents.** `bonnie-deploy` and
`bonnie-rollback`, both defined in `deploy/terraform/ci-deploy.tf`, both
taking a single `sha` parameter with `allowedPattern = ^[0-9a-f]{40}$`. The
IAM policy allows `SendCommand` only on those two document ARNs, and
separately only at an instance tagged `Name = bonnie-dev`. SSM authorises the
document and the instance independently on the same call, so both must pass.

The command text lives in git and is reviewed. A workflow edit cannot change
what runs on the box; it can only choose which of the two documents to invoke
and with which sha. Changing the box's behaviour requires a terraform apply,
which is a different credential and a different review.

The sha is validated three times — by the workflow, by the SSM service against
`allowedPattern`, and by `bonnie-stage` and `bonnie-install` on the box. It is
interpolated into a shell command, so the pattern is the real boundary; the
other two are defence in depth, and the SSM-side one rejects a bad value before
the box ever sees it.

**3. Download and unpack happen as `bonnie-ci`, not root.** A new system
account with no shell and no sudo. `bonnie-stage` verifies the sha256 **before**
unpacking, rejects archives with absolute or traversing paths, and asserts both
binaries are linux/amd64 ELF. Only then does root get involved.

`bonnie-ci` is a *system* account on purpose. `deploy/nftables/bonnie-imds.nft`
rejects IMDS for uid ≥ 1000, so human users cannot borrow the instance role —
but a uid below 1000 can, which is how `bonnie-stage` reads the release bucket
with no stored credential anywhere. The instance role's S3 grant is already
`GetObject` on `releases/*` only; nothing new is needed.

**4. Approval is enforced by IAM, not by YAML.** The deploy job declares
`environment: bonnie-dev`, which carries a required reviewer. The OIDC trust
policy accepts only `sub = repo:superbuilders/clyde:environment:bonnie-dev`.
Deleting the `environment:` line from the workflow therefore does not skip the
approval — it makes the credential request fail, because the token's `sub` no
longer matches. This is the mechanism that implements BONNIE.md's "merging to
master needs sign-off" as something stronger than a convention.

## The unmerged-branch problem

Today's deploy of `f8f03bc` is exactly the failure this guards against. It is on
`mobile-session-viewer`, not `master`; nothing on the box records that, and the
next deploy built from `master` reverts it silently, with no error anywhere.

The build job runs `git merge-base --is-ancestor HEAD origin/master` and fails
if the candidate is not contained in master. Deploying a branch is still
possible — tick `allow_unmerged` — but it is then a deliberate act that prints
a warning naming the branch that needs merging, instead of a thing nobody
noticed.

## Rollback

```
Actions ▸ bonnie deploy ▸ Run workflow ▸ rollback_to = <40-char sha>
```

Skips build, skips S3, invokes `bonnie-rollback`, which runs only
`bonnie-install --activate <sha>`. No network, no bucket, no CI build — the
bytes are already on disk and were verified when first staged. Rollback has to
work when the network or the bucket is the broken thing.

Every deploy prints `previous: /opt/bonnie/versions/<sha>` as the first line of
its SSM output, so the rollback target is in the job summary of the run that
needs rolling back.

## Why the S3 hop

The box could pull the artefact straight from the Actions run. It should not:
that needs a GitHub token on the box, and it makes the artefact's lifetime
GitHub's retention policy rather than ours. S3 is already the release channel —
`scripts/bonnie-release.sh` writes the same layout to the same prefix, and the
instance role already reads it. The CI path adds a producer, not a mechanism.

Artefacts are write-once by convention: the workflow does a `HeadObject` first
and refuses to publish a sha whose key already exists, since re-uploading under
a name that is supposed to be a promise is either a no-op or a lie and there is
no way to tell which from CI. This is a precheck, not an enforcement — the role
can still overwrite. Making it real needs a bucket policy or Object Lock, which
is a change to the bucket and not to this role.

## Current state

Everything below is already applied to `bonnie-dev`. The one thing that is not
done is the merge — see "Remaining".

| piece | state |
|---|---|
| `bonnie-dev-deploy`, `bonnie-dev-rollback` SSM documents | created |
| `bonnie-dev-ci-deploy` IAM role + policy | created |
| `bonnie-ci` account and `/usr/local/bin/bonnie-stage` on the box | bootstrapped |
| GitHub environment `bonnie-dev`, branch policy `master` | created |
| repository variables | set |
| workflow registered and runnable | **no — needs the merge** |

## Setup

### Terraform

`deploy/terraform/ci-deploy.tf` creates the two SSM documents and the
`bonnie-ci-deploy` role and policy. The GitHub OIDC provider already exists in
the account and is looked up, not created.

**Do not run a bare `terraform apply` on this module.** `ami_id` is unset in
`env/dev.tfvars`, so `ami.tf` resolves Canonical's current Ubuntu 24.04 image,
which has been republished since the box was built. A full apply today plans
`aws_instance.web must be replaced` — it rebuilds the dev box, destroying
`/opt`, `/etc/bonnie` and every running tmux session, before anyone notices
what they approved. That drift predates this document and is still unresolved;
pinning `ami_id` is a separate decision.

Apply only the CI resources:

```
terraform init -backend-config=env/dev.backend.hcl
terraform apply -var-file=env/dev.tfvars \
  -target=aws_ssm_document.bonnie_deploy \
  -target=aws_ssm_document.bonnie_rollback \
  -target=aws_iam_role.github_actions_deploy \
  -target=aws_iam_role_policy.github_actions_deploy
```

Check the plan says **0 to destroy** before approving. `-target` pulls in
dependencies, so a single reference to `aws_instance.web` anywhere in this file
drags the instance back into scope and the plan quietly becomes a rebuild. That
is why the policy matches the instance by tag instead of by id — see the
comment on the `AndOnlyAtTheBonnieInstance` statement.

The `cloud-init.yaml` and `main.tf` changes in this PR are **not** applied, and
must not be: they alter `user_data`, which forces instance replacement. They
exist so that a *future, deliberate* instance rebuild provisions `bonnie-ci`
and `bonnie-stage` by itself. The running box was bootstrapped by hand instead.

### The box

`bonnie-setup` runs once, guarded by
`ConditionPathExists=!/srv/bonnie/.setup-complete`, so a running instance never
picks up cloud-init changes. Applied once as root over SSM:

```bash
useradd --system --home-dir /srv/bonnie/ci --shell /usr/sbin/nologin bonnie-ci
install -d -m 0750 -o bonnie-ci -g bonnie-ci /srv/bonnie/ci /srv/bonnie/ci/.ship
install -m 0755 -o root -g root deploy/bonnie-stage /usr/local/bin/bonnie-stage
```

Verified afterwards: uid 997 (below the 1000 threshold the IMDS guard uses),
`sudo -l -U bonnie-ci` reports no sudo at all, and
`runuser -u bonnie-ci -- aws sts get-caller-identity` returns the instance role.

### GitHub

Environment `bonnie-dev`, deployment branch policy restricted to `master`, and
these repository variables (none are secrets — they are ARNs and ids, and
holding them grants nothing):

| variable | value |
|---|---|
| `AWS_DEPLOY_ROLE_ARN` | `arn:aws:iam::565944437804:role/bonnie-dev-ci-deploy` |
| `AWS_REGION` | `us-east-1` |
| `BONNIE_INSTANCE_ID` | `i-023bf2e53984f1998` |
| `BONNIE_RELEASE_BUCKET` | `bonnie-releases-565944437804` |
| `BONNIE_SSM_DOC_DEPLOY` | `bonnie-dev-deploy` |
| `BONNIE_SSM_DOC_ROLLBACK` | `bonnie-dev-rollback` |

The branch policy restricts which branch the *workflow* runs from, not which
commit it builds. Dispatch always happens from `master`; `ref` chooses what
gets built. Those are deliberately separate.

## Verification

Worth repeating after any change to the documents or the policy, because three
of these failed the first time.

```bash
# The injection boundary. Both must be rejected by the SSM service itself,
# before the box is involved.
aws ssm send-command --instance-ids "$I" --document-name bonnie-dev-deploy \
  --parameters 'sha=f8f03bc41cb76731fe5b9dbad585e916eed7d954; touch /tmp/pwned'
aws ssm send-command --instance-ids "$I" --document-name bonnie-dev-deploy \
  --parameters 'sha=F8F03BC41CB76731FE5B9DBAD585E916EED7D954'   # uppercase

# The staging guards, on the box.
runuser -u bonnie-ci -- bonnie-stage 'abc; rm -rf /'   # rejected: not hex
runuser -u bonnie-ci -- bonnie-stage deadbeef          # rejected: not 40 chars
bonnie-stage <valid-sha>                               # rejected: running as root

# The role. Read the decisions, do not infer them from the policy text.
aws iam simulate-principal-policy \
  --policy-source-arn arn:aws:iam::565944437804:role/bonnie-dev-ci-deploy \
  --action-names ssm:SendCommand --resource-arns <arn>
```

`simulate-principal-policy` is not optional. Two bugs in this policy were
invisible by inspection and obvious in simulation:

- A `Deny` intended to make release keys write-once denied the ordinary upload
  too, because an ordinary `PutObject` is not a copy. Removed; the workflow's
  `HeadObject` precheck carries that job, and real immutability belongs in a
  bucket policy or Object Lock.
- Putting the instance-tag condition on the same statement as the document
  ARNs denied *everything*: a condition applies to every resource in its
  statement, and a document has no `ssm:resourceTag/Name`. Split into two
  statements.

A third bug was only findable by running the thing: SSM feeds the document to
`/bin/sh`, which is dash on Ubuntu, so `set -euo pipefail` aborted on line one
with "Illegal option -o pipefail". Both documents now start with an explicit
`#!/usr/bin/env bash`.

## Remaining

1. **Merge to `master`.** Needs AJ per BONNIE.md. `workflow_dispatch` workflows
   are only registered from the default branch, so until this merges the
   workflow does not exist as far as Actions is concerned — this is the single
   thing standing between the current state and a working deploy button.
2. **Fix the required reviewer.** The environment currently lists
   `handlebauer` as a placeholder. `thisistheaj` was rejected: GitHub silently
   drops reviewers without at least write access, and that account has read,
   leaving an *empty* reviewer list — a gate nobody can open, which fails
   closed but also fails permanently. Either grant AJ write and swap him in, or
   confirm the intended approver.
3. **Decide on `ami_id`.** Unrelated to CI, but a live hazard for anyone who
   runs `terraform apply` in this module.

## What this deliberately does not do

- **No deploy on push to master.** The trigger is `workflow_dispatch` only.
  Merging and shipping stay separate decisions; BONNIE.md is explicit that this
  box is the thing people are working inside, and an automatic restart on every
  merge is not a property anyone asked for.
- **No build on the box.** Root never builds in either path, and now neither
  does the box at all in the CI path.
- **No change to `bonnie-install`.** It is the audited piece. The CI path was
  shaped to fit it rather than the other way round.
