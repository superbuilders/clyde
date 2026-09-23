#!/usr/bin/env bash
# Seed the fixture the M3 isolation gate reads for.
#
# The gate asserts that Bob (the e2e user) cannot see Alice's sessions. That
# assertion is only meaningful if Alice HAS a session with known content —
# against a freshly rebuilt box it would otherwise pass vacuously, which is the
# worst possible outcome for a security test.
#
# Bob cannot create or verify this himself; that is the point of the milestone.
# So it is seeded out of band, as root, over SSM.
#
# Usage: scripts/bonnie-m3-fixture.sh
set -euo pipefail

AWS_PROFILE="${AWS_PROFILE:-superbuilders-prod}"
REGION="${AWS_REGION:-us-east-1}"

ALICE="${BONNIE_M3_ALICE:-anthony-beckner}"
MARKER="${BONNIE_M3_MARKER:-alice-private-marker-do-not-leak}"

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
MARKER=$MARKER
HOME_DIR=/srv/bonnie/users/\$ALICE
[ -d "\$HOME_DIR" ] || { echo "\$ALICE is not provisioned"; exit 1; }
S="\$HOME_DIR/code/scratch/.clyde/sessions/2026-09-23T00-00-00_alice"
install -d -o "\$ALICE" -g "\$ALICE" -m 0750 "\$S"
printf '**You:** %s\n' "\$MARKER" >"\$S/2026-09-23T00-00-01.000_user.md"
chown "\$ALICE:\$ALICE" "\$S/2026-09-23T00-00-01.000_user.md"
chmod 0640 "\$S/2026-09-23T00-00-01.000_user.md"
ls -l "\$S"
EOS
)

runner=$(mktemp -t bonnie-fixture)
payload=$(mktemp -t bonnie-fixture-payload)
trap 'rm -f "$runner" "$payload"' EXIT
printf '%s' "$script" >"$payload"

cat >"$runner" <<'PY'
import json, os, subprocess, sys, time
instance, profile, region, script_path = sys.argv[1:5]
env = {**os.environ, "AWS_PROFILE": profile, "AWS_PAGER": ""}
with open(script_path) as fh:
    params = {"commands": [fh.read()]}
with open("/tmp/_bonnie_fixture_params.json", "w") as fh:
    json.dump(params, fh)
cid = subprocess.run(
    ["aws", "ssm", "send-command", "--instance-ids", instance,
     "--document-name", "AWS-RunShellScript", "--region", region,
     "--parameters", "file:///tmp/_bonnie_fixture_params.json",
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
echo "alice    : $ALICE"
python3 "$runner" "$INSTANCE" "$AWS_PROFILE" "$REGION" "$payload"
echo "fixture seeded"
