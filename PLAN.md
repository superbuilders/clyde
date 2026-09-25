# Bonnie — Plan v2 (DRAFT, for review)

> Supersedes `PLAN.md`. Nothing here is approved. Revert point for the `bonnie` repo:
> **`a47a429`** (last docs-only commit, before any code existed).

---

## ⛔ STOP — read this before touching anything

**DO NOT UPDATE THE `clyde` BINARY ON THIS MACHINE.** Not with `make install`, not with
`go install`, not by overwriting whatever is on `$PATH`, not "just to test it." Not until
the entire project is finished and **AJ has personally signed off.**

This is not a style preference. Most of this project's code lands in the `clyde` repo, and
some of it (§7, umask) is in `agent/session` — the agent *you are running inside right
now*. A bad build installed over the working binary takes down the tool being used to
build it, along with every running session on this box.

**The rules, for any agent or human picking this up:**

1. All work happens in a **git worktree**, never in the primary `clyde` checkout.
2. It builds to **differently-named binaries** — `bonnie` (viewer) and `clyde-next`
   (agent) — which live in the worktree's `./bin/` and are **never** copied to `$PATH`,
   `/usr/local/bin`, `~/go/bin`, or `~/bin`.
3. Run them by **absolute path only**.
4. Test against a **scratch `HOME`**, a **non-default port** (`:8788`, not `:8787`), and a
   **dedicated tmux socket** (`-L bonnie-dev`), so a test run cannot touch real
   `~/code/**/.clyde/sessions` data or kill real tmux sessions.
5. `git push` to a branch is fine. Merging to `clyde@master` requires sign-off too,
   because master is what people install from.

If you are unsure whether an action modifies the installed agent: **it does. Don't.**

---

## 0. North star

> **Deploy and gate the session viewer with a login using Cognito and Google.
> Separate users' sessions using Unix as the security system — ~100% on the backend,
> except one added button for sharing a session.**

### Architectural invariants

| # | Invariant |
|---|---|
| **A1** | If the Clyde **agent** changes, Bonnie gets it by `git pull && go build`. |
| **A2** | Bonnie does not own, fork, port, or re-render the viewer's frontend. |
| **A3** | Bonnie does not own the viewer's routes, JSON shapes, or index. |
| **A4** | Agent, viewer, and host/auth config are independently editable and compose unless a contract is substantially broken. |
| **A5** | Sharing changes exactly one thing from the viewer's perspective: **how many directories are in its search path.** |
| **A6** | Bonnie provides **only** an auth gate and a Unix security boundary. |
| **A7** | **Multi-user is the general case. Solo is the degenerate case** — same binary, `auth = none`, one user. |
| **A8** | **Process model: one webserver, privileged, serving all HTML and spawning all agents. One agent per session, running as the owning Unix user, never root.** |

---

## 1. The process model (A8), settled

```
  1  bonnie-web            root, hardened, behind the ALB — serves ALL html for ALL users
  k  tmux servers          one per active user, as that user
  n  clyde agents          one per session, as the owning user, NEVER root
```

`n + k + 1`. The webserver is a normal web application: it can see everything, and it
enforces authorization in application code the way Rails or Django does. That is fine and
normal — **what makes this different from a normal web app is that we have an untrusted
component, the LLM with `run_bash`, and that is what the kernel confines.** Agents run as
the user. The shell cannot escape the uid. Nothing about the security claim depends on the
webserver being unprivileged.

### Why the webserver is privileged, and how that stays safe

It needs `CAP_SETUID`/`CAP_SETGID` to launch agents as other users, and `CAP_CHOWN` to
create files owned by them. That's the whole reason.

Spawning uses Go's native, safe path — credentials are applied **in the child, after fork,
before exec**:

```go
cmd.SysProcAttr = &syscall.SysProcAttr{
    Credential: &syscall.Credential{Uid: u.Uid, Gid: u.Gid, Groups: u.Groups},
}
```

No per-thread credential juggling, no `LockOSThread`, no goroutine that can escape the
boundary. ~10 lines.

Hardening is a standard checklist, not an architecture:

- Bind `127.0.0.1:8080` behind the ALB. **Never** bind `:443` or `:80` directly.
- `CapabilityBoundingSet=CAP_SETUID CAP_SETGID CAP_CHOWN`, `AmbientCapabilities` matching,
  everything else dropped.
- `ProtectSystem=strict`, `ProtectHome=read-only` (with `ReadWritePaths` for the data
  volume), `PrivateTmp`, `ProtectKernelTunables`, `RestrictAddressFamilies=AF_UNIX AF_INET`,
  `SystemCallFilter=@system-service`.
- **Never exec through a shell.** Explicit argv only — the spawn path takes a username as
  input, and `tmux new-session -d -s <name> <cmd>` (`session-viewer/main.go:328`) already
  takes a command string. Audit that call site specifically.

### Rejected alternatives — do not re-litigate

| Option | Why not |
|---|---|
| Unprivileged web + per-user helper daemons over unix sockets | This is v1's `agentd`+`rpc`. Same process count, plus a bespoke protocol to maintain. |
| Unprivileged web + a `serve`/`work` split in one binary | Draft 2 of this plan. Still a herd of resident per-user processes; still a protocol. Deleted. |
| Single process, root, `setfsuid` per request | Go sets credentials process-wide by design (goroutines migrate threads); doing it per-thread needs `LockOSThread` + raw syscalls, and any escaped goroutine reads the wrong user's files. A memory-safety-class bug in the security boundary. |
| Unprivileged web that can read all users' trees | No kernel boundary at all. This is v1. |

---

## 2. Where the work happens

The previous drafts treated `session-viewer` as something to wrap. Wrong. Per A7:

> **`session-viewer` natively satisfies the Bonnie requirements — multi-user, auth,
> sharing, teams. Running locally for one person is the special case that skips auth and
> skips deployment.** The new repo imports it and adds only what is Superbuilders-specific.

Going multi-user → single-user is trivial. Going single-user → multi-user *from outside
the process* is what produces proxies, RPC, per-user supervisors, and eventually a second
frontend. That is v1, precisely.

This also **deletes the share-button problem.** Sharing is native, so the button is just a
button. No plugin dir, no injected `<script>`, no upstream/downstream negotiation.

| Repo | Contains |
|---|---|
| `superbuilders/clyde` | **the work.** `session-viewer` gains auth, principals, privileged spawn, sharing, teams, deploy assets. Apache-2.0, useful to anyone. |
| `superbuilders/bonnie` | **thin.** Timeback Cognito config, panopticon + subrepo preload, read-only AWS `credential_process`, prod tfvars. Private. Target: **a few hundred lines, mostly config.** |

You did ask for the private half — original message, line 5: *"B) a closed source
implementation/extension that can be used at my company."* Only the name `bonnie-sb` was
invented. I also wrongly cut **org/user repo pre-pull** in an earlier draft; your line 12
asks for it explicitly. It's back, in M6.

---

## 3. Facts from the source (verified)

| Fact | Consequence |
|---|---|
| `session-viewer/` is its own Go module | Grows auth + multi-user without touching the agent module. |
| `main.go` is 1628 lines, binds hardcoded `:8787` | Becomes `--listen`. |
| `discoverProjectDirs()` = `$HOME` + `$CWD` + `find ~/code ~/Downloads -maxdepth 4 -name .clyde` | **The entire sharing mechanism, already upstream.** Add a directory → it appears (A5). Needs configurable roots + `-L` to follow share symlinks. |
| Session identity is a directory: `<project>/.clyde/sessions/<id>/*.md` | Unix ownership and ACLs apply directly. |
| Agents are driven via `tmux new-session -d` / `send-keys` / `capture-pane` (`main.go:303-364`) | **Keep tmux.** It gives persistence across webserver restarts and `open-terminal` for free — which is the zero-disruption-deploy requirement, already solved. Agents must be spawned on the *user's* tmux socket, as the user. |
| `agent/session/session.go:67,88,111` hardcode `0755`/`0644` | Breaks group-writable team sessions. See §6. **Agent code — see the STOP notice.** |

---

## 4. Milestones

**Sequencing principle: auth first, deploy immediately after, then everything else on a
live box.** Deploying last maximises what can go wrong at the end. Deploying at M2 means
every milestone from that point is something AJ can log into and try.

### Review gates

**M0–M2 are one work unit.** Build them straight through, deploy, and **stop.** AJ tests
the live box and approves. Every milestone after that is its own gate: build → deploy →
AJ tests → approve → next. No agent starts M4 before M3 is signed off.

---

### M0 — Worktree harness ✅ **DONE** (`clyde@bonnie` b1bdd74)
Worktree off `clyde@master`; `make bonnie` / `make clyde-next` producing `./bin/`;
scratch-`HOME` test script; port `:8788`; tmux socket `-L bonnie-dev`. A `make install`
guard that **refuses to run** in this worktree.
**Exit:** ✅ all verified. Worktree at `~/code/go/clyde/bonnie`; `make bonnie` /
`make clyde-next` → `./bin/`; `make install` refuses; `make bonnie-guard` pins the
installed binary at `4d180460…`. Sandbox isolated on HOME (`~/.bonnie-sandbox`), port
(`:8788`) and tmux socket (`-L bonnie-dev`), verified leaking **0** real sessions and
rendering the **stock** UI via Playwright. Full notes in the worktree's `BONNIE.md`.

**Gotcha found, and now asserted in the script:** the sandbox must live *outside* any git
repo. `scanSessions()` → `detectWorktreeGroup()` runs `git rev-parse --git-common-dir`,
git walks *up*, and a sandbox inside the checkout expands discovery to every sibling
worktree — it served 466 real sessions before this was caught.

**One code change shipped in M0:** `--listen` / `CLYDE_VIEWER_LISTEN` in
`session-viewer/main.go` (~12 lines), replacing the hardcoded `:8787`. Required for port
isolation, and on the M1 path anyway.

### M1 — Auth *(as early as possible)*
OIDC (Cognito + Google federation), PKCE, JWKS, signed cookie, domain allowlist,
`next` open-redirect protection. `auth = none` is the default and is what solo uses.
Plus the **headless e2e login harness** (§5) — it lands here, not later, because every
milestone after this one is verified through it.
**Exit:** login works against `http://localhost:8788` with a real Cognito app client;
out-of-domain refused; Playwright completes a login unattended and reaches the viewer.
*North-star line 1.*

### M2 — Deploy *(as soon as auth exists)* ← **first milestone AJ tests live**
One EC2 box, ALB/ACM/Route53, SSM-only admin, separate EBS + snapshots, Secrets Manager,
systemd unit with the §1 hardening. Still **single Unix user** — everyone who logs in
shares one account. That is intentionally an interim state and the allowlist is restricted
to AJ plus the e2e test user until M3 lands.
**Exit:** `https://bonnie-dev.developer.timeback.com` serves the stock viewer behind Google
login. Merge-to-deploy works. Playwright logs into the **deployed** box unattended.
**AJ tests and signs off here before anything else starts.**

### M3 — Users are Unix users
`Principal` threaded through every filesystem and tmux operation. Privileged spawn via
`SysProcAttr.Credential` on the target user's tmux socket. `provision` (the only root-only
subcommand): idempotent `useradd`, nologin, home skeleton, group, authoritative
`email → username` map that **refuses on ambiguity**.
Solo mode = principal is the invoking user, no privilege needed, **zero behaviour change**;
existing `e2e_fixes_test.go` passes unmodified.
**Exit:** Alice and Bob log into the deployed box and see only their own sessions.
**Gate test: Bob's agent running `cat /home/alice/...` via `run_bash` gets EACCES.** Lands
with the feature, not later. *North-star line 2.*

### M4 — Sharing and the one button

**Split into two phases in this order, for a reason discovered on the box (see below).**

#### M4.1 — Closed by default *(must land before any ACL is granted)*
The umask work from §7, pulled forward from M5. Multi-user agents run `umask 027`, so a
user's tree is `0750`/`0640` and **`other` has no bits anywhere**. `provision` normalises
existing trees. Solo keeps `umask 022` and is byte-identical (§8 item 2).
**Exit:** no path in any user's tree grants anything to `other`; solo regression passes.
**Gate test: a second user with a traverse bit on a home directory can still read nothing.**

#### M4.2 — Sharing
`setfacl` read grant + ancestor traverse bits + default ACLs on live sessions; symlink into
the sharee's configured roots. Share button in the viewer's own UI.
**Exit:** Alice shares; Bob refreshes; it's there, read-only; revoke is complete.
**A5 test: `ssh` in as Bob, run the binary by hand — identical result.**
**Gate tests:** the revoke check must prove access *existed* before revoking, or it passes
vacuously (the M3 fixture lesson). Plus an **overlapping-share fixture**: two shares under
one parent, revoke one, assert the other still works.

#### Why the order is not negotiable
ACLs can only *add* access. They cannot subtract what `other` already has. Measured on the
deployed box: the agent's default `umask 022` makes every directory it creates `0755` and
every file `0644` — world-readable. The **only** thing isolating Alice today is her home
being `0750`, which stops everyone at the door.

A share's ancestor traverse bit punches a hole in exactly that door. So granting Bob one
directory does not expose one directory; it exposes **every `0755` directory Alice's agent
has ever created**, and `0755` means he can list them, not merely guess at them. Verified:
after a correct, complete ACL revoke, the sharee could still read the file — because the
ACL was never what was granting access.

Sharing is only containable on a tree that is closed by default. M4.1 closes it.

### M5 — Teams
Team account with a setgid group-writable tree in everyone's roots. The umask fix moved to
M4.1; team directories select the third regime (`umask 007`) from the same mechanism.
**Exit:** two people drive one team session; both can write.

### M6 — Repo pre-pull
Org- and user-level `repos.toml` with **dirty-tree skip** (uncommitted agent work is
sacred); GitHub App token minting; new-session repo picker seeded from org ∪ user repos.
**Exit:** a fresh box comes up with configured repos present; a dirtied checkout is skipped
and logged, not clobbered.

### M7 — `superbuilders/bonnie`
Timeback Cognito config, panopticon preload, `BonnieTeamReadOnly` IAM + `credential_process`,
prod tfvars. Revert the repo to `a47a429` at this point — there is nothing to put in it
before then. Tag the current head `archive/v1-rpc-agentd` first; the ACL reasoning in
`docs/access-model.md` §3–6 is good and M3/M4 should reuse the *thinking*, not the code.
**Exit:** an engineer logs in with their existing insights SSO account and drives a team
session rooted in panopticon.

### Deleted (all drafts)
Per-user daemons, unix-socket RPC, `SO_PEERCRED`, systemd socket activation, idle-stop,
`serve`/`work` roles, `setfsuid`, a second frontend, `storage.mode = "pooled"`,
`dist/versions/` + per-session version pinning, a TOML config dialect with 12 validation
fixtures, and v1's `internal/{rpc,agentd,acl,audit,layout,principal,repos}`.

**Budget: ~1,000 lines net new in `session-viewer`, a few hundred in `bonnie`.**
If it exceeds that, stop and re-read §1.

---

## 5. Headless e2e auth — how the agent verifies its own work

Lands in **M1** and is used by every milestone after it. Without this, "I tested it" means
"I read the diff," which is how v1 shipped a login you could complete that showed you
nothing.

### The mechanism

A dedicated **Cognito-native test user** (username + password, *not* Google-federated),
driven through the real hosted UI by Playwright:

```
test/e2e/login.ts      → hosted UI, fill username+password, follow callback,
                         assert session cookie, save storageState
test/e2e/state.json    → reused across runs; gitignored
```

Why a native user rather than Google: **Google actively blocks automated sign-in.** Headless
Chrome against `accounts.google.com` hits bot detection, and hardening it is a treadmill.
Cognito's own hosted UI has no such defence, and the federated path converges on the same
callback, the same token verification, and the same cookie — so automating the native user
exercises everything except Google's login form.

**The Google path is verified manually, once per milestone, by AJ.** That is written into
the gate.

### Identity: a test user, not impersonation of AJ

The automation authenticates as `bonnie-e2e@<domain>`, which provisions to its own Unix
user like anyone else. I explicitly do **not** want a mechanism to assume AJ's identity:

- It would be a real impersonation backdoor in a system whose entire purpose is
  separating users. The first thing an attacker looks for.
- It corrupts the audit trail — "AJ started this session" has to stay true.
- It isn't needed. Sharing and isolation tests need *two* identities, not AJ's; M3/M4 use
  `bonnie-e2e-alice` and `bonnie-e2e-bob`.

If a test genuinely needs to observe AJ's own sessions, the answer is for AJ to share one
with the test user — which is the feature under test anyway.

**No `--dev-accept-token` flag, no header-auth bypass, no signed impersonation grant, in
any build.** A debug auth bypass that ships is how this goes wrong, and "it's config-gated"
is not a defence.

### Credentials

Test-user password lives in Secrets Manager (`bonnie/e2e/test-user`) and, locally, in an
untracked `.env.e2e`. Never committed, never in a session transcript, never in argv.

### What the agent must run before claiming a milestone works

1. `make test` — unit + the preserved `e2e_fixes_test.go`.
2. `make e2e` — Playwright against the **deployed** box: log in, list sessions, start a
   session, send a message, see output stream back.
3. From M3: the isolation assertion — log in as `bonnie-e2e-bob`, start a session, have the
   agent `cat` alice's tree, **assert EACCES**.

A milestone is not done until (2) passes against the deployed URL. Not localhost.

---

## 6. Sequencing

```
M0 ──► M1 ──► M2 ──► [GATE: AJ tests live] ──► M3 ──► [GATE] ──► M4.1 ──► M4.2 ──► [GATE] ──► M5 ──► M6 ──► M7
 harness  auth   deploy                        unix users      closed    sharing           teams  repos  bonnie
                                                               by default
```

M4.1 before M4.2 is a hard ordering, not a preference: an ACL grant on a world-readable
tree shares more than it names, and revoking it does not take the access away. See §4 M4.

Auth is as early as it can be (M0 only exists to make development safe). Deploy is
immediately after auth, so M2 onward is always a live, loggable-into system. The first
worker builds M0–M2 and stops; each subsequent milestone is its own build-deploy-test-approve
cycle.

---

## 7. Upstream agent change (the only one)

**umask.** `agent/session/session.go` hardcodes `0755`/`0644`. Hardcoding is the bug; the
mode a session writes with is policy, and policy belongs to the caller.

~6 lines: request `0777`/`0666`, let `umask` decide. One change, three regimes:

- Solo, `umask 022` → `0755`/`0644`. **Identical to today.** (§8 item 2.)
- Multi-user, `umask 027` → `0750`/`0640`. **`other` gets nothing**, which is what makes
  sharing containable — see M4.1.
- Team dir, `umask 007` → `0770`/`0660`. Group-writable, so a second agent can write.

Originally this was scoped to teams only: `0644` is owner-write, group-read, so in a
group-writable team directory Alice starts a session and Bob's agent gets permission denied
on its first write. Team mode is dead on arrival without it.

**That framing was too narrow.** The same hardcoded modes leave `other` readable on every
directory an agent creates, and M4's traverse bits make `other` reachable. So this change is
a prerequisite for *sharing*, not just for teams, and it moves from M5 to M4.1.

Note the two directions. Teams need the mode **loosened** for group; sharing needs it
**tightened** for other. Only a umask-driven design can do both, which is the argument for
deleting the constants rather than editing them.

A latent bug for any shared checkout; defensible upstream on its own merits.

**⚠️ This edits the agent. Build as `clyde-next`, test in the scratch HOME, do not install.**

---

## 8. Definition of done

1. Engineer logs in at `bonnie.developer.timeback.com` with Timeback SSO and uses the
   **stock** viewer — because there is only one viewer.
2. `session-viewer` with no config is still exactly today's solo tool. Regression-tested.
3. Bob's agent cannot read Alice's sessions via `run_bash`.
4. One button shares read-only; revoke is complete. **Complete means the access is gone,
   not that the ACL entry is gone** — which requires a tree that grants nothing to `other`,
   so that the ACL is the only route in (M4.1).
5. Process model matches A8: `n + k + 1`, and **no agent runs as root.**
6. Restarting `bonnie-web` mid-session disturbs nothing — agents live in tmux, not in the
   webserver's process tree. Falls out of A8 rather than being engineered.
7. Every milestone from M2 on was **verified by Playwright against the deployed URL**, and
   signed off by AJ, before the next one started.
8. Net new code within budget (§4).
9. **The `clyde` binary on AJ's machine was never replaced during development**, and the
   merge to `clyde@master` happened only after explicit sign-off.

Items 2, 5, 7, 8 and 9 are what keep v1 from happening again.
