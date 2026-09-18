#!/usr/bin/env bash
# Run the Bonnie build of session-viewer in an isolated sandbox.
#
#   - scratch HOME          so it cannot see or damage real ~/code/**/.clyde/sessions
#   - port 8788             so it cannot collide with the real viewer on 8787
#   - tmux socket bonnie-dev  so it cannot list, drive or kill real tmux sessions
#
# Nothing here touches the installed clyde binary.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# The sandbox MUST live outside any git repository.
#
# scanSessions() calls detectWorktreeGroup() on every discovered dir, which runs
# `git -C <dir> rev-parse --git-common-dir`. git walks UP the tree — so a sandbox
# inside the checkout resolves to the clyde repo and expands the discovery set to
# every sibling worktree. Verified the hard way: it surfaced 466 real sessions
# from hive/, clyde/ and grokbot-ui/. Do not move this back under $HERE.
SANDBOX="${BONNIE_SANDBOX:-$HOME/.bonnie-sandbox}"
PORT="${BONNIE_PORT:-8788}"
TMUX_SOCKET="${BONNIE_TMUX_SOCKET:-bonnie-dev}"

"$HERE/scripts/bonnie-guard.sh"

mkdir -p "$SANDBOX/home/code/demo/.clyde/sessions" "$SANDBOX/bin" "$SANDBOX/tmux"

# Assert the isolation property rather than trusting it.
if git -C "$SANDBOX/home" rev-parse --git-common-dir >/dev/null 2>&1; then
	echo "⛔ sandbox $SANDBOX/home is inside a git repo — it would leak real sessions" >&2
	exit 1
fi

# tmux isolation without a code change: a shim earlier on PATH than the real
# tmux, which forces every invocation onto a dedicated socket. The viewer shells
# out to bare `tmux` (main.go:303-364), so this catches list/new/send/kill/capture
# uniformly and makes it impossible for a dev run to see or kill a real session.
cat >"$SANDBOX/bin/tmux" <<SHIM
#!/usr/bin/env bash
exec /opt/homebrew/bin/tmux -L "$TMUX_SOCKET" "\$@"
SHIM
chmod +x "$SANDBOX/bin/tmux"

# A fixture session so the viewer has something to render on a cold start.
fixture="$SANDBOX/home/code/demo/.clyde/sessions/2026-01-01T00-00-00_fixture"
if [[ ! -d "$fixture" ]]; then
	mkdir -p "$fixture"
	printf '**You:**\n\nfixture session for bonnie M0\n' >"$fixture/2026-01-01T00-00-00.000_user.md"
	printf '**Clyde:**\n\nIf you can read this, the sandbox works.\n' >"$fixture/2026-01-01T00-00-01.000_assistant.md"
fi

echo "sandbox : $SANDBOX/home"
echo "port    : $PORT"
echo "tmux    : -L $TMUX_SOCKET (via shim $SANDBOX/bin/tmux)"
echo "binary  : $HERE/bin/bonnie"
echo

# CRITICAL: run from inside the sandbox, never from the worktree.
# discoverProjectDirs() adds os.Getwd(), and detectWorktreeGroup() expands a
# git worktree to its whole sibling group — so starting here from the checkout
# pulls in every other clyde worktree's real sessions. Verified: it found 467.
cd "$SANDBOX/home"

exec env -i \
	PATH="$SANDBOX/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin" \
	HOME="$SANDBOX/home" \
	USER="${USER:-bonniedev}" \
	TERM="${TERM:-xterm-256color}" \
	TMUX_TMPDIR="$SANDBOX/tmux" \
	CLYDE_VIEWER_LISTEN=":$PORT" \
	"$HERE/bin/bonnie" "$@"
