#!/usr/bin/env bash
# Put GitHub App credentials into the secret the box reads at boot.
#
# The App itself has to be created by a human in a browser — GitHub has no
# API for it — so this script does the half that can be automated: it merges
# the three App fields into the existing secret without disturbing anything
# else in there, and it proves the credential works before saving it.
#
# Usage:
#   scripts/bonnie-github-app.sh --app-id 123456 \
#     --installation-id 7891011 --key ~/Downloads/bonnie.<date>.private-key.pem
#
set -euo pipefail

PROFILE="${AWS_PROFILE:-superbuilders-prod}"
REGION="${AWS_REGION:-us-east-1}"
SECRET="bonnie-dev/github"
APP_ID="" INSTALL_ID="" KEY=""

while [ $# -gt 0 ]; do
  case "$1" in
    --app-id)          APP_ID="$2"; shift 2 ;;
    --installation-id) INSTALL_ID="$2"; shift 2 ;;
    --key)             KEY="$2"; shift 2 ;;
    --secret)          SECRET="$2"; shift 2 ;;
    --profile)         PROFILE="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

for v in APP_ID INSTALL_ID KEY; do
  eval "val=\$$v"
  [ -n "$val" ] || { echo "missing --$(echo "$v" | tr 'A-Z_' 'a-z-')" >&2; exit 2; }
done
[ -r "$KEY" ] || { echo "cannot read key: $KEY" >&2; exit 2; }

export AWS_PROFILE="$PROFILE" AWS_PAGER=""

# Merge, don't overwrite: the secret still carries GITHUB_USER, and on a box
# that has not yet been redeployed it also carries the PAT that the service
# account is still using. Clobbering the whole document would break both.
cur=$(aws secretsmanager get-secret-value --secret-id "$SECRET" \
        --query SecretString --output text --region "$REGION")

merged=$(APP_ID="$APP_ID" INSTALL_ID="$INSTALL_ID" KEY="$KEY" \
  python3 -c '
import json, os, sys
d = json.load(sys.stdin)
d["GITHUB_APP_ID"] = os.environ["APP_ID"]
d["GITHUB_APP_INSTALLATION_ID"] = os.environ["INSTALL_ID"]
with open(os.environ["KEY"]) as fh:
    d["GITHUB_APP_PRIVATE_KEY"] = fh.read()
json.dump(d, sys.stdout)
' <<<"$cur")

# Prove it before saving. A credential that does not work is worse than none:
# the refresher would write a token-shaped string into five homes and every
# clone would fail with an authentication error rather than a missing one.
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
printf '%s' "$merged" >"$tmp/secret.json"
if ! (cd "$(dirname "$0")/../session-viewer" && go run ./cmd/ghapp-check "$tmp/secret.json"); then
  echo "credential check failed — secret NOT updated" >&2
  exit 1
fi

aws secretsmanager put-secret-value --secret-id "$SECRET" \
  --secret-string "file://$tmp/secret.json" --region "$REGION" >/dev/null
echo "updated $SECRET"
echo
echo "next: ./scripts/bonnie-deploy.sh   # ships the timer and mints the first tokens"
