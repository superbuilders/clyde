#!/usr/bin/env bash
# Build a Bonnie release tarball and upload it to S3.
#
# The tarball contains exactly two binaries at its root:
#   bonnie  — the authenticated session viewer (session-viewer module)
#   clyde   — the agent (root module), built as the box's `clyde`
#
# NOTE ON THE STOP RULE: this cross-compiles for linux/amd64 into ./dist and
# uploads it. It never writes ./bin, never writes $PATH, and cannot replace the
# clyde binary on this machine. The artefact is a Linux ELF; it would not even
# execute here.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$HERE"

AWS_PROFILE="${AWS_PROFILE:-superbuilders-prod}"
REGION="${AWS_REGION:-us-east-1}"
BUCKET="${BONNIE_RELEASE_BUCKET:-bonnie-releases-565944437804}"
PREFIX="${BONNIE_RELEASE_PREFIX:-releases}"
VERSION="${BONNIE_VERSION:-$(git rev-parse HEAD)}"

# Refuse to ship a dirty tree: the version is a git SHA, and a dirty tree makes
# that SHA a lie about what is running on the box.
if [[ -n "$(git status --porcelain)" && "${BONNIE_ALLOW_DIRTY:-0}" != "1" ]]; then
	echo "⛔ working tree is dirty; commit first (or BONNIE_ALLOW_DIRTY=1)" >&2
	git status --short >&2
	exit 1
fi

"$HERE/scripts/bonnie-guard.sh"

DIST="$HERE/dist/$VERSION"
rm -rf "$DIST"
mkdir -p "$DIST"

echo "== building linux/amd64 =="
( cd session-viewer && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$DIST/bonnie" . )
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$DIST/clyde" .

# Agent skills travel with the binary. provision copies them into each user's
# ~/.agents/skills, which is the only place skill discovery looks, so a release
# that forgets them leaves every user unable to log in to GitHub (PLAN.md §M6).
echo "== bundling skills =="
cp -R "$HERE/deploy/skills" "$DIST/skills"
# A skill with no SKILL.md is invisible to discovery and silently does nothing,
# which is the kind of failure that surfaces as "the agent doesn't know how".
for d in "$DIST"/skills/*/; do
	[ -f "$d/SKILL.md" ] || { echo "ERROR: $d has no SKILL.md" >&2; exit 1; }
done
ls "$DIST/skills"

file "$DIST/bonnie" "$DIST/clyde" 2>/dev/null || true
ls -la "$DIST"

echo "== packaging =="
TAR="$HERE/dist/$VERSION.tar.gz"
# COPYFILE_DISABLE stops macOS bsdtar from emitting AppleDouble "._" resource
# forks, which otherwise land in /opt/bonnie/current as 501:staff junk.
COPYFILE_DISABLE=1 tar -czf "$TAR" -C "$DIST" bonnie clyde skills
# Fail loudly rather than shipping them silently.
if tar -tzf "$TAR" | grep -q '\._'; then
	echo "ERROR: release tarball contains AppleDouble files" >&2
	tar -tzf "$TAR" >&2
	exit 1
fi
SHA="$(shasum -a 256 "$TAR" | awk '{print $1}')"
printf '%s' "$SHA" >"$TAR.sha256"
echo "sha256: $SHA"

echo "== uploading to s3://$BUCKET/$PREFIX/$VERSION.tar.gz =="
aws --profile "$AWS_PROFILE" --region "$REGION" \
	s3 cp "$TAR" "s3://$BUCKET/$PREFIX/$VERSION.tar.gz"
aws --profile "$AWS_PROFILE" --region "$REGION" \
	s3 cp "$TAR.sha256" "s3://$BUCKET/$PREFIX/$VERSION.tar.gz.sha256"

echo
echo "released $VERSION"
echo "  set release_version = \"$VERSION\" in your tfvars, or redeploy with:"
echo "  scripts/bonnie-deploy.sh $VERSION"
