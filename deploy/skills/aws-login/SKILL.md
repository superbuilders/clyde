---
name: aws-login
description: "Sign this user in to AWS as themselves so the aws CLI works. Use when an aws command fails with an authentication, credential, expired-token or AccessDenied error, or when the user asks to connect, log in to, or authenticate with AWS."
triggers:
  - aws sso
  - aws login
  - aws auth
  - connect aws
  - Unable to locate credentials
  - ExpiredToken
  - InvalidClientTokenId
  - AccessDenied
  - credentials could not be refreshed
---

# Signing a user in to AWS

On this machine each person authenticates to AWS as themselves, through IAM
Identity Center (SSO). There is no shared credential and nothing for you to
look up.

Deliberately, there is also no fallback. This box has an EC2 instance role, and
a user reaching it would silently inherit permissions belonging to the service
rather than to themselves — including the ability to read Bonnie's own secrets.
So access to the instance metadata endpoint is blocked for human accounts, and
`aws` without a login simply fails. That is the design working, not a fault.

Your job is to drive the login and read the code out to them, because they have
no shell here. You are their terminal.

## First, check whether it is already done

```bash
aws sts get-caller-identity
```

If that prints an ARN ending in the user's own name, they are logged in. Say so
and carry on with what they actually asked for.

If it fails, check for an expired session before starting over — a refresh is
much faster than a fresh login:

```bash
aws sso login --profile "$(aws configure list-profiles | head -1)"
```

## First-time setup

If `aws configure list-profiles` is empty, the profile does not exist yet.
Create it with the values the user gives you — never guess an account id or a
role name:

```bash
aws configure sso
```

This is interactive, so run it in tmux exactly as below. Ask the user for the
SSO start URL, the region, the account and the role if you do not already have
them.

## The flow

`aws sso login` blocks while it waits for the browser, so it must run in tmux.
A plain `run_bash` would hang until it timed out and you would never see the
code.

**Step 1 — start it.**

```bash
tmux kill-session -t awslogin 2>/dev/null
tmux new-session -d -s awslogin \
  "aws sso login --profile <profile> --no-browser"
sleep 4
tmux capture-pane -t awslogin -p
```

`--no-browser` matters. Without it the CLI tries to launch a browser this
machine does not have, and prints the URL in a less predictable place.

**Step 2 — read the URL and code out.**

The pane will show something like:

```
Browser will not be automatically opened.
Please visit the following URL:

https://device.sso.us-east-1.amazonaws.com/?user_code=ABCD-EFGH

Then enter the code:

ABCD-EFGH
```

Tell the user, with the values you actually read from the pane:

> Go to **https://device.sso.us-east-1.amazonaws.com/** and enter the code
> **ABCD-EFGH**. Tell me when you've done it.

Never invent or guess the code. Read it from the pane every time — it differs
on every attempt and expires in a few minutes.

**Step 3 — confirm.**

Once they say they are done:

```bash
tmux capture-pane -t awslogin -p
aws sts get-caller-identity
```

Repeat if it has not landed yet; the poll can lag a few seconds behind the
browser. If the code expired, kill the session and start again from step 1.

## Then verify it actually works

Do not report success on `get-caller-identity` alone — that only proves the
token exists. Check it can do something:

```bash
aws sts get-caller-identity --query Arn --output text
aws s3 ls 2>&1 | head -5
```

An `AccessDenied` here is a *different* problem from a login failure: the user
is authenticated but their role lacks the permission. Say which of the two it
is, rather than restarting the login.

## When the session expires

SSO sessions are short-lived by design, usually eight to twelve hours. An
`ExpiredToken` on a command that worked this morning is normal. Re-run
`aws sso login --profile <profile>`; it does not require the full setup again.

## Rules

- Never ask the user to paste an access key or a secret key, and never write
  credentials to `~/.aws/credentials` yourself. Long-lived keys on a shared box
  are exactly what SSO exists to avoid.
- Never print a token, an access key or a session token. The device code is
  fine to show — it is useless without the user's own browser session.
- Never run `aws sso login` outside tmux.
- Never try to reach `169.254.169.254` to work around a failure, and never
  suggest the user ask for that block to be lifted. Inheriting the instance
  role would give them Bonnie's own access rather than their own, which is the
  precise thing this setup prevents.
- The login belongs to this user alone. Do not attempt it for anyone else, and
  do not read another user's `~/.aws`.
