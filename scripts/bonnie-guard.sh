#!/usr/bin/env bash
# Assert the installed clyde binary has not been touched by Bonnie development.
#
# M0 of the Bonnie plan records a baseline hash. Every milestone must be able to
# prove the binary on this machine is still the one that was there at the start.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASELINE_FILE="$HERE/.bonnie-baseline"

installed="$(command -v clyde || true)"
if [[ -z "$installed" ]]; then
	echo "⚠️  no 'clyde' on PATH — nothing to protect, but that is unexpected"
	exit 0
fi

actual="$(shasum -a 256 "$installed" | awk '{print $1}')"

if [[ ! -f "$BASELINE_FILE" ]]; then
	echo "recording baseline for $installed"
	printf '%s  %s\n' "$actual" "$installed" >"$BASELINE_FILE"
	echo "$actual"
	exit 0
fi

expected="$(awk '{print $1}' "$BASELINE_FILE")"

if [[ "$actual" != "$expected" ]]; then
	cat <<EOF

  ⛔ THE INSTALLED CLYDE BINARY HAS CHANGED.

     path:     $installed
     expected: $expected
     actual:   $actual

  Bonnie development must never replace the installed agent (BONNIE.md).
  Stop, tell AJ, and do not continue until this is explained.

EOF
	exit 1
fi

echo "✓ installed clyde unchanged ($installed)"
