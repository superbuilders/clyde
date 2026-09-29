---
name: github-login
description: "Sign this user in to GitHub so git and gh can reach their private repositories. Use when git or gh fails with an authentication error, when a clone of a private repo fails, or when the user asks to connect, log in to, or authenticate with GitHub."
triggers:
  - gh auth
  - github login
  - github auth
  - connect github
  - authentication failed
  - could not read Username
  - Repository not found
  - permission denied
---

# Signing a user in to GitHub

On this machine each person authenticates to GitHub as themselves. There is no
shared credential, and nothing for you to look up: the user runs through a
device-code flow once, and afterwards `git` and `gh` can reach exactly the
repositories that person can reach — across every organisation they belong to.

Your job is to drive that flow and read the code out to them, because they have
no shell here. You are their terminal.

## First, check whether it is already done

```bash
gh auth status
```

If that reports a logged-in account, there is nothing to do. Say so and carry
on with whatever they actually asked for. Do not re-run the login.

## The flow

`gh auth login` blocks while it waits for the user to authorise in a browser,
so it must run in tmux. A plain `run_bash` would hang until it timed out and
you would never see the code.

**Step 1 — start it.**

```bash
tmux kill-session -t ghlogin 2>/dev/null
tmux new-session -d -s ghlogin \
  "BROWSER=true gh auth login --hostname github.com --git-protocol https \
   --web --scopes 'repo,read:org,workflow'"
sleep 4
tmux capture-pane -t ghlogin -p
```

`BROWSER=true` matters. It points gh at a command that does nothing and
succeeds, so gh stops trying to launch a real browser on a machine that has
none.

**Step 2 — answer the git prompt.**

The pane will show:

```
? Authenticate Git with your GitHub credentials? (Y/n)
```

Accept the default. This is what makes `git` work as well as `gh`, so do not
answer no.

```bash
tmux send-keys -t ghlogin Enter
sleep 4
tmux capture-pane -t ghlogin -p
```

**Step 3 — read the code out, then start the poll.**

The pane now shows something like:

```
! First copy your one-time code: B84B-627B
Press Enter to open github.com in your browser...
```

Send the second Enter. gh does not begin polling until you do — if you skip
it, the user will authorise successfully and gh will sit there forever.

```bash
tmux send-keys -t ghlogin Enter
```

Then tell the user, with the code you actually read from the pane:

> Go to **https://github.com/login/device** and enter the code **B84B-627B**.
> Tell me when you've done it.

Never invent or guess the code. Read it from the pane every time — it is
different on every attempt and expires in a few minutes.

**Step 4 — confirm.**

Once they say they are done:

```bash
gh auth status
```

Repeat if it has not landed yet; the poll can lag a few seconds behind the
browser. If the code expired, kill the session and start again from step 1.

## Then verify it actually works

Do not report success on `gh auth status` alone. Check that the token can
really see repositories:

```bash
gh repo list --limit 5
```

## When an organisation's repos are missing

If login succeeded but repositories from one organisation are absent, that
organisation has a third-party application access policy and has not approved
the GitHub CLI. The user cannot fix this from here. Tell them:

> Your account is connected, but **<org>** restricts third-party applications.
> An owner of that organisation needs to approve **GitHub CLI** under
> Settings → Third-party Access → OAuth app policy.

This is a GitHub-side permission, not something to work around. Do not try
another credential.

## Cloning after login

Clone into `~/code/`, because that is where this machine looks for projects:

```bash
gh repo clone <org>/<repo> ~/code/<repo>
```

A fresh clone appears as a project as soon as it exists — it does not need a
conversation in it first. The user can then start a session there.

## Rules

- Never ask the user to paste a token, and never write one to a file yourself.
  gh stores its own credential correctly; a hand-placed token gets stale and
  then fails in ways that look like something else.
- Never print a token. The device code is fine to show — it is useless without
  the user's own browser session, and they need to read it.
- Never run `gh auth login` outside tmux.
- The login belongs to this user alone. Do not attempt it for anyone else, and
  do not read another user's gh configuration.
