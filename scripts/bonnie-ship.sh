#!/usr/bin/env bash
# Build and ship Bonnie from the box itself, as yourself.
#
# This is the unprivileged half of self-shipping. It builds in your own home,
# then hands the artefacts to `sudo bonnie-install`, which is the only thing
# here that touches root. Run it from a clyde session — there is no SSH.
#
# Usage:
#   scripts/bonnie-ship.sh          # build HEAD and activate it
#   scripts/bonnie-ship.sh --build  # build only, print the sha, change nothing
#
# Requires membership in bonnie-ship; without it the sudo step is refused and
# the build is simply left on disk.
#
# Contrast with bonnie-release.sh, which cross-compiles on a laptop and uploads
# a checksummed artefact to S3. This path never leaves the box: faster, but the
# binary has no attestation beyond the tree it came from. Use the S3 path for
# anything that is not this dev box.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$HERE"

command -v go >/dev/null 2>&1 || export PATH="$PATH:/usr/local/go/bin"
command -v go >/dev/null 2>&1 || {
	echo "no go toolchain — run bonnie-deploy.sh once to install it" >&2
	exit 1
}

# A dirty tree would produce a binary whose sha names a commit it is not
# actually built from, and /opt/bonnie/versions/<sha> would then be a lie.
# Refuse rather than silently mislabel a rollback target.
if [ -n "$(git status --porcelain)" ]; then
	echo "working tree is dirty — commit first, or the sha will not match the binary" >&2
	git status --short >&2
	exit 1
fi
SHA="$(git rev-parse HEAD)"

BUILD="$HERE/.ship/$SHA"
rm -rf "$BUILD"
mkdir -p "$BUILD"

echo "== building $SHA =="
# Native build: this box is the target, so no GOOS/GOARCH juggling. -trimpath
# to match what bonnie-release.sh produces.
(cd session-viewer && go build -trimpath -o "$BUILD/bonnie" .)
go build -trimpath -o "$BUILD/clyde" .

# Skills travel with the binaries; provision copies them into every home.
cp -R "$HERE/deploy/skills" "$BUILD/skills"
for d in "$BUILD"/skills/*/; do
	[ -f "$d/SKILL.md" ] || {
		echo "ERROR: $(basename "$d") has no SKILL.md" >&2
		exit 1
	}
done

echo "built:"
ls -la "$BUILD"

if [ "${1:-}" = "--build" ]; then
	echo
	echo "not activated. to ship it:"
	echo "  sudo bonnie-install $SHA $BUILD"
	exit 0
fi

echo "== installing =="
sudo bonnie-install "$SHA" "$BUILD"
