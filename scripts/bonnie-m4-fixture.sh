#!/usr/bin/env bash
# Seed the fixture the M4 sharing gate reads for.
#
# The gate asserts that a share works, that revoking it takes the access away,
# and that revoking one of two overlapping shares leaves the other alive. All
# three need Alice to own real directories with known content:
#
#   ~/code/shared-a/  marker + a session   } both under ~/code, which is what
#   ~/code/shared-b/  marker + a session   } makes the overlap case meaningful
#
# The overlap matters because a share of ~/code/shared-a needs a traverse bit
# on ~/code, and so does ~/code/shared-b. A revoke that strips the shared
# ancestor breaks the surviving share silently — nothing errors, the other
# user's project simply vanishes. Two directories under one parent is the
# smallest fixture that can catch it.
#
# Bob cannot create any of this himself; that is the point of the milestone.
# Seeded out of band as root, over SSM. Idempotent: safe to re-run, and it
# clears any ACLs left by a previous run so the gate never starts from a
# half-shared state that would make its first assertion pass vacuously.
#
# Usage: scripts/bonnie-m4-fixture.sh
set -euo pipefail

AWS_PROFILE="${AWS_PROFILE:-superbuilders-prod}"
REGION="${AWS_REGION:-us-east-1}"

ALICE="${BONNIE_M4_ALICE:-anthony-beckner}"
ALICE_EMAIL="${BONNIE_M4_ALICE_EMAIL:-anthony.beckner@superbuilders.school}"
BOB="${BONNIE_M4_BOB:-bonnie-e2e}"
MARKER="${BONNIE_M4_MARKER:-alice-shared-marker-m4}"

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

script=$(
	cat <<EOS
set -eu
ALICE=$ALICE
ALICE_EMAIL=$ALICE_EMAIL
BOB=$BOB
MARKER=$MARKER
HOME_DIR=/srv/bonnie/users/\$ALICE
[ -d "\$HOME_DIR" ] || { echo "\$ALICE is not provisioned"; exit 1; }
id "\$BOB" >/dev/null 2>&1 || { echo "\$BOB is not provisioned"; exit 1; }

# Start from no shares at all. A leftover grant would let the gate's opening
# "Bob cannot see it yet" assertion pass for the wrong reason, or its closing
# revoke assertion pass without ever having granted anything.
setfacl -R -x "u:\$BOB" "\$HOME_DIR" 2>/dev/null || true
setfacl -R -d -x "u:\$BOB" "\$HOME_DIR" 2>/dev/null || true
rm -rf "/srv/bonnie/users/\$BOB/shared"

for p in shared-a shared-b; do
  S="\$HOME_DIR/code/\$p/.clyde/sessions/2026-09-24T00-00-00_alice"
  install -d -o "\$ALICE" -g "\$ALICE" -m 0750 "\$S"
  F="\$S/2026-09-24T00-00-01.000_user.md"
  printf '**You:** %s in %s\n' "\$MARKER" "\$p" >"\$F"
  chown "\$ALICE:\$ALICE" "\$F"
  chmod 0640 "\$F"
  # -R, because install -d applies -o/-g to the final component only: the
  # intermediate directories it creates stay root-owned. Alice must own these
  # outright, since POSIX lets only the owner set an ACL — a root-owned share
  # directory would make the grant fail as her.
  chown -R "\$ALICE:\$ALICE" "\$HOME_DIR/code/\$p"
done

# Close the tree using the product's own code path, not a hand-rolled chmod.
#
# install -d applies its mode to the final component only, so the directories
# just created are 0755 and would defeat the check below. Rather than fixing
# that with chmod here — which would leave the gate asserting the fixture's
# tidiness rather than the system's — run provision, whose closeToOther is the
# M4.1 mechanism that must hold for real users too. Idempotent by design.
/opt/bonnie/current/bonnie provision --email "\$ALICE_EMAIL" >/dev/null

# The tree must grant nothing to other, or the gate proves nothing: on a
# world-readable tree a traverse bit exposes everything below the home and a
# revoke does not take the access back. This is M4.1, asserted rather than
# assumed, because provision is what enforces it and provision runs elsewhere.
OPEN=\$(find "\$HOME_DIR" -xdev \( -type d -o -type f \) -perm /o=rwx | wc -l)
if [ "\$OPEN" -ne 0 ]; then
  echo "REFUSING: \$OPEN paths under \$HOME_DIR still grant access to other."
  echo "Run provision (M4.1) before seeding; sharing is not containable yet."
  find "\$HOME_DIR" -xdev \( -type d -o -type f \) -perm /o=rwx | head -10
  exit 1
fi

echo "alice tree closed to other: ok"
ls -ld "\$HOME_DIR/code/shared-a" "\$HOME_DIR/code/shared-b"
EOS
)

runner=$(mktemp -t bonnie-m4-fixture)
payload=$(mktemp -t bonnie-m4-fixture-payload)
trap 'rm -f "$runner" "$payload"' EXIT
printf '%s' "$script" >"$payload"

cat >"$runner" <<'PY'
import json, os, subprocess, sys, time
instance, profile, region, script_path = sys.argv[1:5]
env = {**os.environ, "AWS_PROFILE": profile, "AWS_PAGER": ""}
with open(script_path) as fh:
    params = {"commands": [fh.read()]}
with open("/tmp/_bonnie_m4_fixture_params.json", "w") as fh:
    json.dump(params, fh)
cid = subprocess.run(
    ["aws", "ssm", "send-command", "--instance-ids", instance,
     "--document-name", "AWS-RunShellScript", "--region", region,
     "--parameters", "file:///tmp/_bonnie_m4_fixture_params.json",
     "--query", "Command.CommandId", "--output", "text"],
    capture_output=True, text=True, env=env, check=True).stdout.strip()
for _ in range(60):
    r = subprocess.run(
        ["aws", "ssm", "get-command-invocation", "--command-id", cid,
         "--instance-id", instance, "--region", region, "--output", "json"],
        capture_output=True, text=True, env=env)
    if r.returncode == 0:
        d = json.loads(r.stdout)
        if d["Status"] in ("Success", "Failed", "TimedOut", "Cancelled"):
            print(d.get("StandardOutputContent", ""), end="")
            err = d.get("StandardErrorContent", "")
            if err:
                print("--- stderr ---\n" + err, file=sys.stderr)
            sys.exit(0 if d["Status"] == "Success" else 1)
    time.sleep(2)
print("timed out waiting for SSM", file=sys.stderr)
sys.exit(1)
PY

echo "instance : $INSTANCE"
echo "alice    : $ALICE <$ALICE_EMAIL>"
echo "bob      : $BOB"
python3 "$runner" "$INSTANCE" "$AWS_PROFILE" "$REGION" "$payload"
echo "fixture seeded"
