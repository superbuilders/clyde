#!/usr/bin/env bash
# Assert the installed clyde binary was not built from Bonnie development.
#
# BONNIE.md's rule is that Bonnie development must never replace the installed
# agent. M0 implemented that as a pinned SHA-256 of the binary, which was the
# wrong mechanism: clyde is under active development in other worktrees, so the
# installed binary changes legitimately and often. A hash pin cannot tell "AJ
# installed a new clyde from master" from "a Bonnie agent clobbered it", so it
# fired on every routine update and the only available fix was to re-baseline.
# A check whose normal outcome is to silence it protects nothing.
#
# Provenance is the property we actually care about, and Go records it: every
# binary built from a repo carries vcs.revision and vcs.modified in its build
# info. So ask the binary where it came from, and fail only if it came from
# here — a commit that exists on this branch but not on master.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Each check is a function so that an early "this one is fine" cannot skip the
# checks after it. The original script used bare `exit 0` for the pass cases,
# which made every check below the first one unreachable.
check_installed_agent() {
installed="$(command -v clyde || true)"
if [[ -z "$installed" ]]; then
	echo "⚠️  no 'clyde' on PATH — nothing to protect, but that is unexpected"
	return 0
fi

info="$(go version -m "$installed" 2>/dev/null || true)"
revision="$(awk '$1=="build" && $2 ~ /^vcs\.revision=/ {sub(/^vcs\.revision=/,"",$2); print $2}' <<<"$info")"
modified="$(awk '$1=="build" && $2 ~ /^vcs\.modified=/ {sub(/^vcs\.modified=/,"",$2); print $2}' <<<"$info")"

if [[ -z "$revision" ]]; then
	# Fail closed, and not merely out of caution: in this repo, missing build
	# info is the *signature* of the thing we are guarding against. This
	# checkout is a git worktree (.git is a file, not a directory) and Go
	# declines to stamp vcs.* when building from one, so binaries built here
	# carry no provenance — while a normal build from the main checkout does.
	# Treating "cannot verify" as a pass would therefore wave through exactly
	# the case that matters. Verified by building ./bin/clyde-next and
	# checking: no vcs.revision, with or without -buildvcs=true or GOWORK=off.
	cat <<EOF

  ⛔ THE INSTALLED CLYDE BINARY HAS NO BUILD PROVENANCE.

     path: $installed

  Go stamps vcs.revision into binaries built from a normal checkout, but not
  into ones built from a git worktree — which is what this Bonnie checkout is.
  An unstamped binary is therefore most likely a build from here.

  If you installed clyde by some other means that strips build info, say so
  and this check needs widening. Otherwise: restore a build from master.

EOF
	return 1
fi

short="${revision:0:12}"

if ! git -C "$HERE" cat-file -e "$revision^{commit}" 2>/dev/null; then
	# Built from a commit this checkout has never seen. It cannot be Bonnie
	# work, which by definition lives here, but it is worth surfacing.
	echo "⚠️  installed clyde is from unknown commit $short — not from this worktree"
	return 0
fi

# The test is "does the binary contain commits unique to this branch", not "is
# it an ancestor of this branch". Bonnie merges master regularly, so every
# master commit is an ancestor of bonnie and the ancestor test would flag them
# all. Commits on master are always legitimate, whatever else contains them.
baseline="${BONNIE_GUARD_BASE:-master}"
if git -C "$HERE" merge-base --is-ancestor "$revision" "$baseline" 2>/dev/null; then
	subject="$(git -C "$HERE" log -1 --format='%s' "$revision" | cut -c1-60)"
	echo "✓ installed clyde is from $baseline at $short ($subject)"
	if [[ "$modified" == "true" ]]; then
		echo "  note: built from a dirty tree, so its contents are not fully pinned"
	fi
	return 0
fi

cat <<EOF

  ⛔ THE INSTALLED CLYDE BINARY WAS BUILT FROM BONNIE WORK.

     path:     $installed
     revision: $short
     subject:  $(git -C "$HERE" log -1 --format='%s' "$revision" 2>/dev/null | cut -c1-60)

  This commit is not on '$baseline', so the agent on this machine contains
  unreleased Bonnie changes. BONNIE.md: development must never replace the
  installed agent.

  Restore it with a build from $baseline, then tell AJ.

EOF
	return 1
}

# ---------------------------------------------------------------------------
# Assert the viewer frontend is still recognisably upstream's.
#
# BONNIE.md A2 forbids porting, forking, or rewriting session-viewer's
# frontend, and v1 shipped a broken login partly by rewriting this file. The
# original guard asserted a byte-for-byte match, which is a stricter rule than
# A2 actually states and one that sharing cannot satisfy: the Share button has
# to live somewhere.
#
# What A2 protects is upstream's work, so the invariant is *additive-only*:
# Bonnie may add UI, and may not remove or rewrite what upstream wrote. A
# deletion is the signature of a port or a rewrite; an insertion is not. The
# line budget keeps "adds a button" from drifting into "adds a second
# frontend" without anyone noticing.
# ---------------------------------------------------------------------------
check_viewer_frontend() {
	local VIEWER_HTML="session-viewer/static/index.html"
	local UPSTREAM_REF="171f610"
	local MAX_ADDED=260

	[[ -f "$HERE/$VIEWER_HTML" ]] || return 0

	if ! git -C "$HERE" cat-file -e "$UPSTREAM_REF:$VIEWER_HTML" 2>/dev/null; then
		echo "⚠️  cannot resolve $UPSTREAM_REF:$VIEWER_HTML — skipping viewer check"
		return 0
	fi

	if git -C "$HERE" diff --quiet "$UPSTREAM_REF" -- "$VIEWER_HTML"; then
		echo "✓ viewer frontend unchanged from upstream ($VIEWER_HTML)"
		return 0
	fi

	local stat added removed
	stat="$(git -C "$HERE" diff --numstat "$UPSTREAM_REF" -- "$VIEWER_HTML")"
	added="$(awk '{print $1}' <<<"$stat")"
	removed="$(awk '{print $2}' <<<"$stat")"

	if [[ "${removed:-0}" -ne 0 ]]; then
		cat <<EOF

  ⛔ THE VIEWER FRONTEND HAS LINES REMOVED FROM UPSTREAM.

     file:     $VIEWER_HTML
     upstream: $UPSTREAM_REF
     added:    $added
     removed:  $removed

  BONNIE.md A2: do not port, fork, or rewrite session-viewer's frontend.
  Additions are allowed — Bonnie has its own UI to contribute — but removing
  or rewriting upstream's lines is what A2 forbids, and is how v1 broke login.

  See exactly what was removed with:
    git diff $UPSTREAM_REF -- $VIEWER_HTML | grep '^-'

EOF
		return 1
	fi

	if [[ "${added:-0}" -gt "$MAX_ADDED" ]]; then
		cat <<EOF

  ⛔ THE VIEWER FRONTEND HAS GROWN BEYOND THE ADDITIVE BUDGET.

     file:     $VIEWER_HTML
     added:    $added lines (budget $MAX_ADDED)

  Nothing upstream was removed, so this is not a rewrite — but this much new
  UI is no longer "Bonnie adds a button". Either the addition belongs
  upstream, or the budget needs raising deliberately rather than silently.

EOF
		return 1
	fi

	echo "✓ viewer frontend is upstream plus $added additive line(s), nothing removed"
	return 0
}

rc=0
check_installed_agent || rc=1
check_viewer_frontend || rc=1
exit "$rc"
