#!/usr/bin/env bash
# Roll the deployed box onto an already-released version, in place, over SSM.
#
# bonnie-release.sh builds and uploads; this activates. Keeping them separate
# means a release is never automatically live, and a rollback is the same
# command with an older sha.
#
# It also syncs the systemd unit and provisions the Unix accounts named in
# provision_emails. Both live in cloud-init, but cloud-init's write_files is a
# per-INSTANCE module: a user_data change applied with `terraform apply` stops
# and starts the box without changing its instance id, so write_files never
# re-runs and the unit on disk silently stays at whatever first booted. Shipping
# the unit here keeps the repo the single source of truth for a running box.
#
# The box has no public IP and no SSH, so all of this goes through SSM
# RunShellScript. Note /bin/sh on AL2023 is dash: no `set -o pipefail`.
#
# Usage:
#   scripts/bonnie-deploy.sh <git-sha>
#   scripts/bonnie-deploy.sh            # deploys release_version from dev.tfvars
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TFVARS="$HERE/deploy/terraform/env/dev.tfvars"
AWS_PROFILE="${AWS_PROFILE:-superbuilders-prod}"
REGION="${AWS_REGION:-us-east-1}"
BUCKET="${BONNIE_BUCKET:-bonnie-releases-565944437804}"
PREFIX="${BONNIE_PREFIX:-releases}"

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
	VERSION="$(sed -n 's/^release_version *= *"\(.*\)"/\1/p' "$TFVARS")"
fi
[[ -n "$VERSION" ]] || {
	echo "no version given and none in $TFVARS" >&2
	exit 1
}

# Resolve the instance by tag rather than hardcoding an id: the box has been
# replaced before, and a stale id fails in confusing ways.
INSTANCE="${BONNIE_INSTANCE:-}"
if [[ -z "$INSTANCE" ]]; then
	INSTANCE="$(aws --profile "$AWS_PROFILE" --region "$REGION" ec2 describe-instances \
		--filters 'Name=tag:Name,Values=bonnie-dev' 'Name=instance-state-name,Values=running' \
		--query 'Reservations[].Instances[].InstanceId' --output text | awk '{print $1}')"
fi
[[ -n "$INSTANCE" ]] || {
	echo "could not find a running instance tagged bonnie-dev" >&2
	exit 1
}

echo "instance : $INSTANCE"
echo "version  : $VERSION"

# Verify the artifact exists before touching the box, so a typo cannot leave
# /opt/bonnie/current dangling.
aws --profile "$AWS_PROFILE" --region "$REGION" \
	s3api head-object --bucket "$BUCKET" --key "$PREFIX/$VERSION.tar.gz" >/dev/null

UNIT="$HERE/deploy/systemd/bonnie-web.service"
UNIT_B64="$(base64 <"$UNIT" | tr -d '\n')"
# Self-shipping assets. Same reason as the unit: these live in cloud-init's
# write_files, which is a per-INSTANCE module, so an existing box never sees a
# change to them unless it is shipped here.
INSTALL_B64="$(base64 <"$HERE/deploy/bonnie-install" | tr -d '\n')"
SUDOERS_B64="$(base64 <"$HERE/deploy/sudoers.d/bonnie-ship" | tr -d '\n')"
IMDS_NFT_B64="$(base64 <"$HERE/deploy/nftables/bonnie-imds.nft" | tr -d '\n')"
IMDS_UNIT_B64="$(base64 <"$HERE/deploy/systemd/bonnie-imds-guard.service" | tr -d '\n')"
PROVISION_EMAILS="$(sed -n 's/^provision_emails *= *"\(.*\)"/\1/p' "$TFVARS")"
echo "unit     : $UNIT"
echo "users    : ${PROVISION_EMAILS:-(none)}"

script=$(
	cat <<EOS
set -eu
VERSION=$VERSION
UNIT_B64=$UNIT_B64
INSTALL_B64=$INSTALL_B64
SUDOERS_B64=$SUDOERS_B64
IMDS_NFT_B64=$IMDS_NFT_B64
IMDS_UNIT_B64=$IMDS_UNIT_B64
PROVISION_EMAILS="$PROVISION_EMAILS"
BUCKET=$BUCKET
PREFIX=$PREFIX
REGION=$REGION
prev=\$(readlink /opt/bonnie/current || echo none)
echo "previous: \$prev"
mkdir -p "/opt/bonnie/versions/\$VERSION"
tmp=\$(mktemp -d)
aws s3 cp "s3://\$BUCKET/\$PREFIX/\$VERSION.tar.gz" "\$tmp/r.tar.gz" --region "\$REGION"
aws s3 cp "s3://\$BUCKET/\$PREFIX/\$VERSION.tar.gz.sha256" "\$tmp/r.sha256" --region "\$REGION"
( cd "\$tmp" && echo "\$(cat r.sha256)  r.tar.gz" | sha256sum -c - )
tar -xzf "\$tmp/r.tar.gz" -C "/opt/bonnie/versions/\$VERSION"
rm -rf "\$tmp"
chmod 0755 "/opt/bonnie/versions/\$VERSION/bonnie" "/opt/bonnie/versions/\$VERSION/clyde"
# The tarball is built on macOS and carries 501:staff. Harmless for the
# binaries, which are chmod'd above, but the skills tree is read by provision
# and copied into every home — it should be owned by root like everything else
# under /opt, not by a uid that happens not to exist here.
chown -R root:root "/opt/bonnie/versions/\$VERSION"
ln -sfn "/opt/bonnie/versions/\$VERSION" /opt/bonnie/current
ln -sfn /opt/bonnie/current/clyde /usr/local/bin/clyde

# The unit, from the repo. Written before provisioning so a failed provision
# leaves a box whose unit and binary at least agree.
echo "$UNIT_B64" | base64 -d >/etc/systemd/system/bonnie-web.service
chmod 0644 /etc/systemd/system/bonnie-web.service
systemctl daemon-reload

# Self-shipping: the installer, the sudoers rule, the group and the toolchain.
echo "\$INSTALL_B64" | base64 -d >/usr/local/sbin/bonnie-install
chown root:root /usr/local/sbin/bonnie-install
chmod 0755 /usr/local/sbin/bonnie-install

# sudoers is validated before it is installed, never after. A malformed
# drop-in makes sudo refuse to run at all, and there is no SSH to fix it with.
echo "\$SUDOERS_B64" | base64 -d >/tmp/bonnie-ship.sudoers
if visudo -cf /tmp/bonnie-ship.sudoers >/dev/null; then
  install -o root -g root -m 0440 /tmp/bonnie-ship.sudoers /etc/sudoers.d/bonnie-ship
  echo "sudoers: installed"
else
  echo "ERROR: refusing to install an invalid sudoers drop-in"; exit 1
fi
rm -f /tmp/bonnie-ship.sudoers
getent group bonnie-ship >/dev/null || groupadd --system bonnie-ship

# Deny the instance role to human users (uid >= 1000). Applied before the web
# service is restarted below, so no agent is spawned while the hole is open.
mkdir -p /etc/bonnie
echo "\$IMDS_NFT_B64" | base64 -d >/etc/bonnie/imds.nft
chmod 0644 /etc/bonnie/imds.nft
echo "\$IMDS_UNIT_B64" | base64 -d >/etc/systemd/system/bonnie-imds-guard.service
chmod 0644 /etc/systemd/system/bonnie-imds-guard.service
systemctl daemon-reload
if systemctl enable --now bonnie-imds-guard.service 2>&1; then
  systemctl restart bonnie-imds-guard.service
  echo "imds guard: \$(systemctl is-active bonnie-imds-guard.service)"
else
  echo "WARN: IMDS guard failed to start — users can reach the instance role"
fi

GO_VERSION=1.24.0
GO_SHA256=dea9ca38a0b852a74e81c26134671af7c0fbe65d81b0dc1c5bfe22cf7d4c8858
if [ ! -x /usr/local/go/bin/go ]; then
  t=\$(mktemp -d)
  if curl -fsSL -o "\$t/go.tgz" "https://go.dev/dl/go\$GO_VERSION.linux-amd64.tar.gz" &&
     echo "\$GO_SHA256  \$t/go.tgz" | sha256sum -c - >/dev/null; then
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "\$t/go.tgz"
  else
    echo "WARN: go download or checksum failed; the box cannot self-ship"
  fi
  rm -rf "\$t"
fi
printf 'export PATH=\$PATH:/usr/local/go/bin:\$HOME/go/bin\n' >/etc/profile.d/go.sh
chmod 0644 /etc/profile.d/go.sh
/usr/local/go/bin/go version 2>/dev/null || echo "WARN: no go toolchain"

# The GitHub CLI. In cloud-init's package list for new boxes, installed here
# for ones that predate it — this is how a user authenticates to GitHub at all
# (PLAN.md §M6), and without it the skill provision installs cannot run.
if ! command -v gh >/dev/null 2>&1; then
  DEBIAN_FRONTEND=noninteractive apt-get install -y -q gh >/dev/null 2>&1 \\
    || echo "WARN: could not install gh; users cannot log in to GitHub"
fi
command -v gh >/dev/null 2>&1 && gh --version | head -1

# Unix accounts for the named users. Idempotent, and non-fatal: a box with a
# current binary and one unprovisioned user is better than a failed rollout,
# and the unprovisioned user fails closed rather than sharing an account.
install -d -m 0751 -o root -g root /srv/bonnie/users
# POSIX word-splitting on commas, not bash arrays: SSM's AWS-RunShellScript
# runs this under dash.
for e in \$(echo "\$PROVISION_EMAILS" | tr ',' ' '); do
  [ -n "\$e" ] || continue
  BONNIE_SERVICE_HOME=/srv/bonnie/home /opt/bonnie/current/bonnie provision --email "\$e" \\
    || echo "WARN: provision \$e failed"
done
# The agent must be able to load its config, or sessions produce no output.
# Checked by reading the file as the service account, NOT by running clyde:
# clyde parses no flags, so \`clyde --version\` is taken as a PROMPT and
# silently spends a model call (and spawns an agent) on every deploy.
sudo -u bonnie test -r /srv/bonnie/home/.clyde/config || { echo "agent config unreadable"; exit 1; }
echo "agent config: ok"
systemctl restart bonnie-web.service
sleep 4
systemctl is-active bonnie-web.service
curl -sf -o /dev/null -w "healthz=%{http_code}\n" localhost:8080/healthz
EOS
)

runner=$(mktemp -t bonnie-ssm)
cat >"$runner" <<'PY'
import json, os, subprocess, sys, time
instance, profile, region, script_path = sys.argv[1:5]
env = {**os.environ, "AWS_PROFILE": profile, "AWS_PAGER": ""}
with open(script_path) as fh:
    params = {"commands": [fh.read()]}
with open("/tmp/_bonnie_deploy_params.json", "w") as fh:
    json.dump(params, fh)
cid = subprocess.run(
    ["aws", "ssm", "send-command", "--instance-ids", instance,
     "--document-name", "AWS-RunShellScript", "--region", region,
     "--parameters", "file:///tmp/_bonnie_deploy_params.json",
     "--query", "Command.CommandId", "--output", "text"],
    capture_output=True, text=True, env=env, check=True).stdout.strip()
for _ in range(120):
    r = subprocess.run(
        ["aws", "ssm", "get-command-invocation", "--command-id", cid,
         "--instance-id", instance, "--region", region, "--output", "json"],
        capture_output=True, text=True, env=env)
    if r.returncode == 0:
        d = json.loads(r.stdout)
        if d["Status"] in ("Success", "Failed", "TimedOut", "Cancelled"):
            print(d.get("StandardOutputContent", ""))
            err = d.get("StandardErrorContent", "")
            if err:
                print("--- stderr ---", err, sep="\n", file=sys.stderr)
            sys.exit(0 if d["Status"] == "Success" else 1)
    time.sleep(2)
print("timed out waiting for SSM", file=sys.stderr)
sys.exit(1)
PY

payload=$(mktemp -t bonnie-payload)
printf '%s' "$script" >"$payload"
trap 'rm -f "$runner" "$payload"' EXIT

python3 "$runner" "$INSTANCE" "$AWS_PROFILE" "$REGION" "$payload"
