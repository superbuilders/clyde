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

# ---------------------------------------------------------------------------
# Assert the viewer frontend is still upstream's. BONNIE.md forbids porting,
# forking, or rewriting session-viewer's frontend: index.html stays untouched.
# v1 shipped a broken login partly by rewriting this file, so pin it.
# ---------------------------------------------------------------------------
VIEWER_HTML="session-viewer/static/index.html"
UPSTREAM_REF="171f610"

if [[ -f "$HERE/$VIEWER_HTML" ]]; then
	if ! git -C "$HERE" cat-file -e "$UPSTREAM_REF:$VIEWER_HTML" 2>/dev/null; then
		echo "⚠️  cannot resolve $UPSTREAM_REF:$VIEWER_HTML — skipping viewer check"
	elif git -C "$HERE" diff --quiet "$UPSTREAM_REF" -- "$VIEWER_HTML"; then
		echo "✓ viewer frontend unchanged from upstream ($VIEWER_HTML)"
	else
		cat <<EOF

  ⛔ THE VIEWER FRONTEND HAS BEEN MODIFIED.

     file:     $VIEWER_HTML
     upstream: $UPSTREAM_REF

  BONNIE.md: do not port, fork, or rewrite session-viewer's frontend.
  Revert with: git checkout $UPSTREAM_REF -- $VIEWER_HTML

EOF
		exit 1
	fi
fi
