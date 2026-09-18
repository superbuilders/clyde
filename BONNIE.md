# BONNIE — read this before you touch anything

You are on the `bonnie` branch of `superbuilders/clyde`. This branch turns
`session-viewer` into a multi-user, authenticated, deployable service. The plan lives at
`panopticon/repos/bonnie/PLAN.md`. **Read it before writing code.**

---

## ⛔ DO NOT UPDATE THE INSTALLED `clyde` BINARY

Not with `make install` (it's a tripwire — it refuses), not `go install`, not by copying
`bin/*` anywhere, not "just to test it." **Not until the whole project is done and AJ has
personally signed off.**

Work on this branch touches `agent/session` — the agent *you are running inside right
now*. A bad build installed over the working binary takes down the tool being used to
build it, plus every running session on this machine.

### The rules

1. **Worktree only.** This one: `~/code/go/clyde/bonnie`. Never the primary checkout at
   `~/code/go/clyde/clyde`.
2. **Differently-named binaries.** `make bonnie` → `./bin/bonnie` (the viewer).
   `make clyde-next` → `./bin/clyde-next` (the agent). Never copied to `$PATH`,
   `/usr/local/bin`, `~/go/bin`, or `~/bin`.
3. **Absolute paths only** when running them.
4. **Sandbox only** for dev runs: `make bonnie-dev`. See below.
5. **`make bonnie-guard`** asserts the installed binary is byte-identical to the M0
   baseline. Run it before and after any work session. If it fails, stop and tell AJ.
6. Pushing the `bonnie` branch is fine. **Merging to `master` needs sign-off** — master is
   what people install from.

If you are unsure whether an action modifies the installed agent: **it does. Don't.**

---

## The sandbox (`make bonnie-dev`)

Three isolations, all of them load-bearing:

| | how | why |
|---|---|---|
| **Data** | `HOME=~/.bonnie-sandbox/home` | so it cannot read or write real `~/code/**/.clyde/sessions` |
| **Port** | `CLYDE_VIEWER_LISTEN=:8788` | so it cannot collide with the real viewer on `:8787` |
| **tmux** | a shim on `PATH` forcing `-L bonnie-dev` | so it cannot list, drive, or **kill** real tmux sessions |

### The sandbox must live outside any git repository

It is at `~/.bonnie-sandbox`, deliberately **not** inside the worktree. `scanSessions()`
calls `detectWorktreeGroup()` on every discovered directory, which runs
`git -C <dir> rev-parse --git-common-dir`. **git walks up the tree.** A sandbox inside the
checkout therefore resolves to the clyde repo and expands discovery to every sibling
worktree.

This is not hypothetical — it happened during M0, and the "isolated" viewer served **466
real sessions** from `hive/`, `clyde/` and `grokbot-ui/`. `scripts/bonnie-dev.sh` now
asserts the property and refuses to start otherwise. Do not move the sandbox back under
the worktree.

### Verifying isolation

```
curl -s localhost:8788/api/sessions | \
  python3 -c "import json,sys; s=json.load(sys.stdin)['sessions']; \
  print('leaked:', len([x for x in s if '.bonnie-sandbox' not in x['cwd']]))"
```

Must print `leaked: 0`. If it doesn't, something in discovery reaches outside the sandbox —
find it before writing another line.

---

## What M0 changed

Deliberately almost nothing, so the baseline is trustworthy:

- `session-viewer/main.go` — `--listen` / `CLYDE_VIEWER_LISTEN`, replacing the hardcoded
  `:8787`. ~12 lines. Required for sandbox port isolation, and on the M1 path anyway.
- `Makefile` — `bonnie`, `clyde-next`, `bonnie-dev`, `bonnie-guard`, `bonnie-clean`, and
  an `install` target that refuses.
- `scripts/bonnie-dev.sh`, `scripts/bonnie-guard.sh`.
- `.gitignore` — `bin/`, `.bonnie-sandbox`, `.bonnie-baseline`.

**No auth, no multi-user, no sharing yet.** That's M1 onward.

## M0 exit criteria — verified

- [x] `./bin/bonnie` serves the stock viewer, stock UI, against a fixture tree
- [x] Sandbox leaks zero real sessions (after fixing the git-worktree hole)
- [x] Real tmux sessions untouched; real viewer on `:8787` undisturbed
- [x] `make install` refuses
- [x] Installed `clyde` byte-identical to baseline
      `4d180460c53aff8d230f8095d481cc67f94aa5a279a1a7479c43e1bb5eb7f9aa`
