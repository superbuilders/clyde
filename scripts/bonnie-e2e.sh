#!/usr/bin/env bash
# Run the Bonnie e2e auth harness (PLAN.md §5).
#
# Credentials come from Secrets Manager (bonnie-dev/e2e/test-user) or, locally,
# from test/e2e/.env.e2e. They are never committed, never in argv, never echoed.
#
# Usage:
#   scripts/bonnie-e2e.sh                       # against localhost:8788
#   BONNIE_URL=https://bonnie-dev.developer.timeback.com scripts/bonnie-e2e.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
E2E="$HERE/test/e2e"
AWS_PROFILE="${AWS_PROFILE:-superbuilders-prod}"
SECRET_ID="${BONNIE_E2E_SECRET:-bonnie-dev/e2e/test-user}"

export BONNIE_URL="${BONNIE_URL:-http://localhost:8788}"

if [[ -f "$E2E/.env.e2e" ]]; then
	echo "credentials: $E2E/.env.e2e"
	set -a
	# shellcheck disable=SC1091
	source "$E2E/.env.e2e"
	set +a
else
	echo "credentials: Secrets Manager $SECRET_ID (profile $AWS_PROFILE)"
	creds="$(aws --profile "$AWS_PROFILE" secretsmanager get-secret-value \
		--secret-id "$SECRET_ID" --query SecretString --output text)"
	BONNIE_E2E_USERNAME="$(printf '%s' "$creds" | python3 -c 'import json,sys;print(json.load(sys.stdin)["username"])')"
	BONNIE_E2E_PASSWORD="$(printf '%s' "$creds" | python3 -c 'import json,sys;print(json.load(sys.stdin)["password"])')"
	export BONNIE_E2E_USERNAME BONNIE_E2E_PASSWORD
	unset creds
fi

echo "target     : $BONNIE_URL"
echo "test user  : $BONNIE_E2E_USERNAME"
echo

cd "$E2E"
if [[ ! -d node_modules ]]; then
	echo "installing playwright…"
	npm install --silent
	npx playwright install chromium
fi

exec npx playwright test "$@"
