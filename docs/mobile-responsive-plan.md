# Mobile responsiveness for the Bonnie session viewer

## Where the code is

One file: `session-viewer/static/index.html` (Tailwind v4 browser build + daisyUI 5 +
Alpine 3, all from CDN). It is **embedded** into the `bonnie` binary via `//go:embed
static/*` (`session-viewer/main.go:32`), so a change here is not live until the binary is
rebuilt and reinstalled.

## Deployment plan (we are working *inside* Bonnie)

The box self-ships. `scripts/bonnie-ship.sh` builds `bonnie` + `clyde` + `skills/` into
`.ship/<sha>` as me, then calls `sudo bonnie-install <sha> <dir>`, which copies the
artefacts to `/opt/bonnie/versions/<sha>`, flips the `/opt/bonnie/current` symlink and
restarts `bonnie-web.service`. I am in the `bonnie-ship` group, so the sudo rule applies.

Preconditions / hazards, all checked:

1. **The tree must be clean and committed.** `bonnie-ship.sh` refuses a dirty tree —
   otherwise `/opt/bonnie/versions/<sha>` would be named for a commit it wasn't built
   from. So: commit first, ship second.
2. **The deployed sha is `origin/master` (838288c).** The worktree I started in was 73
   commits behind — the stale `master` had no `deploy/`, no sharing, no multi-user. I
   reset it to `origin/master` so the diff I ship is exactly my change.
3. **Restarting the service does not kill this session.** Agents live in tmux outside
   the unit's cgroup, and `RuntimeDirectoryPreserve=yes` keeps `/run/bonnie/<user>`
   (the tmux sockets) alive across the restart. Self-shipping from inside a session is
   explicitly a supported path. Cost is a few refused HTTP connections.
4. **Never touch the installed `clyde` binary by hand** (BONNIE.md). `bonnie-install` is
   the only thing allowed to, and only as part of an atomic symlink flip.
5. **Rollback is one command:** `sudo bonnie-install --activate 838288c`.

Order of operations:

```
go build ./... && go vet            # compile check (no Go changes expected)
go test ./session-viewer/...        # no test reads index.html, but prove it
python3 -m http.server / sandbox run on :8788   # eyeball at phone widths
git commit                          # clean tree required by the ship script
scripts/bonnie-ship.sh              # build + sudo bonnie-install + restart
curl -sS localhost:8080 | grep ...  # confirm the new markup is being served
```

## The changes

### 1. Sidebar → off-canvas drawer under `md`

- Sidebar becomes `fixed inset-y-0 left-0 z-40 w-[85vw] max-w-sm` with
  `-translate-x-full` when closed, `md:static md:translate-x-0 md:w-96`.
- New Alpine state `sidebarOpen` (false by default; irrelevant at `md`+ because the
  element is `static` there).
- A dimmed backdrop (`md:hidden`) closes it on tap.
- `selectSession()` closes the drawer, so picking a session reveals the transcript.
- A `md:hidden` hamburger sits in the main column — in the session top bar when one is
  selected, and in a dedicated mobile header when none is.

### 2. Overlays sized for a phone

- Modals (takeover / share / worktree): `p-4` on the backdrop, `max-h-[90vh]
  overflow-y-auto`, `p-4 sm:p-6` inside, so they never exceed the viewport.
- Dropdown menus: `max-h-[70vh] overflow-y-auto`; the filter menu gets a width that
  fits a 320px screen.
- Toast spans the width on mobile instead of hugging a corner.
- Row `⋮` buttons are hover-revealed (`opacity-0 group-hover:…`) which is unreachable on
  touch — forced visible under `md`.

### 3. Chat usable on mobile

- `h-screen` → `h-[100dvh]` so mobile browser chrome doesn't clip the composer.
- Bubbles get `max-w-[85%] break-words`; `.msg-content` gets `overflow-wrap:anywhere`
  and `pre { max-width: 100% }` so long paths/URLs/tool output can't force a horizontal
  scroll of the whole page.
- Inputs/selects/textarea at `font-size:16px` under 640px — anything smaller makes iOS
  Safari zoom the page on focus and never zoom back.
- Composer: tighter padding, send button shrinks to an icon-ish size on mobile.
