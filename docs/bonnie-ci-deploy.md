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
IAM policy names those two document ARNs and the one instance ARN, so a
`SendCommand` naming any other document or instance is denied by IAM.

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

Artefacts are immutable: the workflow refuses to publish a sha whose key already
exists, since re-uploading under a name that is supposed to be a promise is
either a no-op or a lie and there is no way to tell which from CI.

## Setup

Terraform (`deploy/terraform/ci-deploy.tf`) creates the two SSM documents, the
`bonnie-ci-deploy` role and its policy, and outputs the values below. The GitHub
OIDC provider already exists in the account and is looked up, not created.

```
terraform init -backend-config=env/dev.backend.hcl
terraform apply
```

In GitHub, create environment `bonnie-dev` with AJ as a required reviewer, then
set these repository variables (none are secrets — they are ARNs and ids, and
holding them grants nothing):

| variable | value |
|---|---|
| `AWS_DEPLOY_ROLE_ARN` | `terraform output github_actions_role_arn` |
| `AWS_REGION` | `us-east-1` |
| `BONNIE_INSTANCE_ID` | `terraform output bonnie_instance_id` |
| `BONNIE_RELEASE_BUCKET` | `bonnie-releases-565944437804` |
| `BONNIE_SSM_DOC_DEPLOY` | `bonnie-dev-deploy` |
| `BONNIE_SSM_DOC_ROLLBACK` | `bonnie-dev-rollback` |

### Bootstrapping the existing box

`bonnie-setup` runs once, guarded by `ConditionPathExists=!/srv/bonnie/.setup-complete`,
so the running instance will not pick up the new account or `bonnie-stage` from
cloud-init. Apply them once, as root over SSM, on the current box:

```bash
useradd --system --home-dir /srv/bonnie/ci --shell /usr/sbin/nologin bonnie-ci
install -d -m 0750 -o bonnie-ci -g bonnie-ci /srv/bonnie/ci /srv/bonnie/ci/.ship
install -m 0755 -o root -g root deploy/bonnie-stage /usr/local/bin/bonnie-stage
```

Verify before trusting the path end to end:

```bash
runuser -u bonnie-ci -- aws sts get-caller-identity   # must return the instance role
runuser -u bonnie-ci -- bonnie-stage <a-known-sha>    # must stage without sudo
```

A replacement instance gets all of this from cloud-init and needs no bootstrap.

## What this deliberately does not do

- **No deploy on push to master.** The trigger is `workflow_dispatch` only.
  Merging and shipping stay separate decisions; BONNIE.md is explicit that this
  box is the thing people are working inside, and an automatic restart on every
  merge is not a property anyone asked for.
- **No build on the box.** Root never builds in either path, and now neither
  does the box at all in the CI path.
- **No change to `bonnie-install`.** It is the audited piece. The CI path was
  shaped to fit it rather than the other way round.
