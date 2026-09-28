import { test, expect } from "@playwright/test";
import { bonnieShare, ssm } from "./ssm";

// PLAN.md §M4 — the sharing gate.
//
//   "Alice shares; Bob refreshes; it's there, read-only; revoke is complete."
//
// We authenticate as Bob (bonnie-e2e). Alice is the operator account. Alice's
// half runs over SSM against the deployed `bonnie share` subcommand, because
// Bob being unable to grant himself access is precisely what is under test.
//
// SEED THE FIXTURE FIRST: scripts/bonnie-m4-fixture.sh.
//
// The unit of sharing is a conversation, not a project (M4.2 review). That
// changes what this file asserts in one important way: it is no longer enough
// that the shared thing appears and the unshared thing does not. The sibling
// conversation — same project, same .clyde/sessions directory, one level
// away — must stay invisible, and so must the project itself. A project-level
// grant would pass every other assertion here.
//
// Three things this file is careful about, all learned the hard way:
//
//   1. Prove access exists BEFORE revoking it. "Bob cannot see it" passes
//      trivially against a share that never happened, a typo'd id, or a grant
//      that silently failed. A revoke assertion is only meaningful if it
//      follows an assertion that the thing was there a moment earlier. This
//      is the M3 vacuity lesson.
//
//   2. Revoke one of two OVERLAPPING shares. Both conversations live under
//      one .clyde/sessions, so both need a traverse bit on it. A revoke that
//      strips the shared ancestor breaks the surviving share silently —
//      nothing errors, the other conversation simply disappears.
//
//   3. Ask the kernel, not the web layer, whenever the claim is about access
//      rather than about display. The API could be filtering correctly over a
//      filesystem that is wide open.

const ALICE = "anthony-beckner";
const BOB = "bonnie-e2e";
const PROJECT_A = `/srv/bonnie/users/${ALICE}/code/shared-a`;
const PROJECT_B = `/srv/bonnie/users/${ALICE}/code/shared-b`;
const SHARED = "conv-one";
const SIBLING = "conv-two";
const ALICE_HOME = `/srv/bonnie/users/${ALICE}`;

const sessionPath = (cwd: string, id: string) => `${cwd}/.clyde/sessions/${id}`;

type Session = { id: string; cwd: string; project?: string; shared_by?: string };

/** Every conversation the deployed viewer shows Bob, as (cwd, id) pairs. */
async function bobSessions(page: import("@playwright/test").Page): Promise<Session[]> {
  // Rescan first: the session list is served from a cache filled by a
  // background scanner, so a share granted a moment ago is not visible until
  // the tree is re-read.
  await page.request.post("/api/sessions/scan");
  const res = await page.request.get("/api/sessions?days=0");
  expect(res.status(), "GET /api/sessions must be 200").toBe(200);
  const body = await res.json();
  return (Array.isArray(body) ? body : (body.sessions ?? [])) as Session[];
}

function has(list: Session[], cwd: string, id: string): boolean {
  return list.some((s) => s.cwd === cwd && s.id === id);
}

function describeList(list: Session[]): string {
  return list.map((s) => `${s.cwd} :: ${s.id}`).join("\n") || "(none)";
}

/** Bob's project list, which must never gain the owner's project. */
async function bobProjects(page: import("@playwright/test").Page): Promise<string[]> {
  const res = await page.request.get("/api/projects");
  expect(res.status(), "GET /api/projects must be 200").toBe(200);
  const body = await res.json();
  const list = Array.isArray(body) ? body : (body.projects ?? []);
  return list.map((p: { path?: string; cwd?: string }) => p.path ?? p.cwd ?? "");
}

const grant = (cwd: string, id: string) =>
  bonnieShare(`--owner ${ALICE} --sharee ${BOB} --cwd ${cwd} --session ${id}`);
const revoke = (cwd: string, id: string) =>
  bonnieShare(`--owner ${ALICE} --sharee ${BOB} --cwd ${cwd} --session ${id} --revoke`);

test.describe.configure({ mode: "serial" });

test.afterAll(() => {
  // Leave the box in the state the fixture creates, whatever happened above.
  // A leftover grant would make the next run's opening assertion pass for the
  // wrong reason.
  for (const cwd of [PROJECT_A, PROJECT_B]) {
    for (const id of [SHARED, SIBLING]) revoke(cwd, id);
  }
});

test("a shared conversation appears for the sharee, is read-only, and revoke takes it away", async ({
  page,
}) => {
  // ── Before: Bob can see none of it ───────────────────────────────────────
  const before = await bobSessions(page);
  expect(
    has(before, PROJECT_A, SHARED),
    `the conversation was already visible — the fixture did not reset:\n${describeList(before)}`,
  ).toBe(false);

  // ── Alice grants one conversation ────────────────────────────────────────
  const granted = grant(PROJECT_A, SHARED);
  expect(granted.status, `bonnie share failed:\n${granted.stdout}${granted.stderr}`).toBe("Success");

  // ── Bob refreshes and it is there ────────────────────────────────────────
  // This assertion is what makes every negative assertion below mean anything.
  const afterGrant = await bobSessions(page);
  expect(
    has(afterGrant, PROJECT_A, SHARED),
    `the shared conversation never became visible to ${BOB}:\n${describeList(afterGrant)}`,
  ).toBe(true);

  // ── And it is attributed, not smuggled into one of Bob's own projects ────
  const shown = afterGrant.find((s) => s.cwd === PROJECT_A && s.id === SHARED)!;
  expect(
    shown.shared_by,
    "a conversation reached through a share must say who shared it, or Bob cannot tell it from his own",
  ).toBeTruthy();
  expect(
    shown.project,
    "a shared conversation must not be grouped under the owner's project name, which Bob cannot open",
  ).toBe(`Shared by ${shown.shared_by}`);

  // ── Sharing ONE conversation shares only that conversation ───────────────
  // This is the claim conversation-level sharing exists to make, and the only
  // assertion here that a project-level grant would fail.
  expect(
    has(afterGrant, PROJECT_A, SIBLING),
    "the sibling conversation in the same project became visible: this is a project share wearing a conversation's clothes",
  ).toBe(false);

  // The kernel agrees: Bob cannot even list the directory the two share.
  const listSiblings = ssm(
    `if runuser -u ${BOB} -- sh -c 'ls ${PROJECT_A}/.clyde/sessions' >/dev/null 2>&1; ` +
      `then echo OUTCOME=LISTED; else echo OUTCOME=DENIED; fi`,
  );
  expect(
    listSiblings.stdout,
    "Bob could enumerate Alice's conversations — the corridor granted read where it should grant only traverse",
  ).toContain("OUTCOME=DENIED");

  // Nor read the project's source, which is the cost project sharing used to
  // impose for handing over a single transcript.
  const readProject = ssm(
    `if runuser -u ${BOB} -- sh -c 'ls ${PROJECT_A}' >/dev/null 2>&1; ` +
      `then echo OUTCOME=LISTED; else echo OUTCOME=DENIED; fi`,
  );
  expect(readProject.stdout, "sharing a conversation exposed the project's files").toContain(
    "OUTCOME=DENIED",
  );

  // Nor does the project appear as somewhere Bob can work.
  const projects = await bobProjects(page);
  expect(projects, "the owner's project appeared in Bob's project list").not.toContain(PROJECT_A);
  expect(projects, "Alice's home became visible").not.toContain(ALICE_HOME);

  // ── It really is readable, not merely listed ─────────────────────────────
  const read = ssm(
    `if runuser -u ${BOB} -- sh -c 'cat ${sessionPath(PROJECT_A, SHARED)}/*.md' >/dev/null 2>&1; ` +
      `then echo OUTCOME=READ; else echo OUTCOME=DENIED; fi`,
  );
  expect(read.stdout, "the shared conversation is listed but not actually readable").toContain(
    "OUTCOME=READ",
  );

  // ── It is read-only ──────────────────────────────────────────────────────
  // A share must not become a place Bob can start an agent: he cannot write,
  // so the session would fail to record itself.
  const newSession = await page.request.post("/api/sessions/new", { data: { cwd: PROJECT_A } });
  expect(
    newSession.status(),
    "a read-only share must refuse a new session rather than starting one that cannot write",
  ).toBe(403);

  // And the kernel agrees, independently of what the web layer decided.
  // Assert on an explicit outcome word rather than an exit code: a failed
  // redirect exits 2 under dash and 1 under bash, so a hardcoded code makes
  // this pass or fail on which shell the box happens to use.
  const write = ssm(
    `if runuser -u ${BOB} -- sh -c 'echo x > ${sessionPath(PROJECT_A, SHARED)}/should-not-exist' 2>/dev/null; ` +
      `then echo OUTCOME=WROTE; else echo OUTCOME=DENIED; fi`,
  );
  expect(write.stdout, "Bob was able to write into a read-only share").toContain("OUTCOME=DENIED");

  // ── A sharee is not a re-sharer ──────────────────────────────────────────
  // Bob can read this conversation, so a check written against "can you see
  // it" rather than "do you own it" would let him pass it on — and Alice's
  // list of who can read her transcript would stop being the truth.
  const reshare = await page.request.post("/api/shares", {
    data: { cwd: PROJECT_A, session_id: SHARED, sharee_email: "someone.else@superbuilders.school" },
  });
  expect(
    reshare.status(),
    "a sharee was able to re-share someone else's conversation (or got a 403, which confirms it exists)",
  ).toBe(404);

  // ── Revoke, and the access is actually gone ──────────────────────────────
  const revoked = revoke(PROJECT_A, SHARED);
  expect(revoked.status, `revoke failed:\n${revoked.stdout}${revoked.stderr}`).toBe("Success");

  const afterRevoke = await bobSessions(page);
  expect(
    has(afterRevoke, PROJECT_A, SHARED),
    `the conversation was still listed after revoke:\n${describeList(afterRevoke)}`,
  ).toBe(false);

  // Complete means the access is gone, not that the ACL entry is gone
  // (PLAN.md §8 item 4). Ask the kernel, as Bob, not the web layer.
  const readAfter = ssm(
    `if runuser -u ${BOB} -- sh -c 'cat ${sessionPath(PROJECT_A, SHARED)}/*.md' >/dev/null 2>&1; ` +
      `then echo OUTCOME=READ; else echo OUTCOME=DENIED; fi`,
  );
  expect(
    readAfter.stdout,
    "Bob could still read the revoked conversation — revocation removed the ACL but not the access",
  ).toContain("OUTCOME=DENIED");

  // ── And it leaves no trace in Bob's home ─────────────────────────────────
  // The link lives at ~/shared/<owner>/<project>/<id>, so removing only the
  // link leaves two named directories behind — a permanent record of who
  // shared with Bob and what their projects are called, including shares that
  // were revoked and project names he was never otherwise told. Nothing else
  // ever deletes them, so they accumulate for the life of the account.
  //
  // This is the sharee-side twin of the stale-corridor-bit rule, and it was
  // found by eyeballing the box after a green run: every other assertion in
  // this file passed with the debris sitting there.
  const residue = ssm(
    `find /srv/bonnie/users/${BOB}/shared -mindepth 1 2>/dev/null | wc -l`,
  );
  expect(
    residue.stdout.trim().split("\n").pop(),
    "revoke left directories behind in the sharee's home",
  ).toBe("0");
});

test("revoking one conversation leaves an overlapping one working", async ({ page }) => {
  // Both conversations sit in the same .clyde/sessions, so both grants depend
  // on the same traverse bits all the way up. This is the case that breaks
  // silently, and it is tighter than the old project-level version: the
  // shared ancestor is now one directory away instead of three.
  const a = grant(PROJECT_A, SHARED);
  expect(a.status, `granting the first conversation failed:\n${a.stdout}${a.stderr}`).toBe("Success");
  const b = grant(PROJECT_A, SIBLING);
  expect(b.status, `granting the second conversation failed:\n${b.stdout}${b.stderr}`).toBe("Success");

  const both = await bobSessions(page);
  expect(has(both, PROJECT_A, SHARED), `the first conversation never appeared:\n${describeList(both)}`).toBe(true);
  expect(has(both, PROJECT_A, SIBLING), `the second conversation never appeared:\n${describeList(both)}`).toBe(true);

  // Revoke only the first. The second must survive.
  const revoked = revoke(PROJECT_A, SHARED);
  expect(revoked.status, `revoke failed:\n${revoked.stdout}${revoked.stderr}`).toBe("Success");

  const after = await bobSessions(page);
  expect(has(after, PROJECT_A, SHARED), "the revoked conversation is still visible").toBe(false);
  expect(
    has(after, PROJECT_A, SIBLING),
    `revoking one share broke an unrelated one: the corridor they share was stripped\n${describeList(after)}`,
  ).toBe(true);

  // And it is genuinely readable, not merely listed.
  const read = ssm(
    `if runuser -u ${BOB} -- sh -c 'cat ${sessionPath(PROJECT_A, SIBLING)}/*.md' >/dev/null 2>&1; ` +
      `then echo OUTCOME=READ; else echo OUTCOME=DENIED; fi`,
  );
  expect(read.stdout, "the surviving share is listed but no longer readable").toContain("OUTCOME=READ");
});

test("a conversation in an unshared project stays invisible throughout", async ({ page }) => {
  // Sharing out of one project must say nothing about another. Kept separate
  // from the assertions above so that a failure here points at cross-project
  // leakage rather than at the grant/revoke logic.
  const g = grant(PROJECT_A, SHARED);
  expect(g.status, `grant failed:\n${g.stdout}${g.stderr}`).toBe("Success");

  const list = await bobSessions(page);
  expect(has(list, PROJECT_A, SHARED), "the grant did not take effect").toBe(true);
  expect(
    has(list, PROJECT_B, SHARED),
    `a conversation with the same id in an unshared project became visible:\n${describeList(list)}`,
  ).toBe(false);
  expect(
    has(list, PROJECT_B, SIBLING),
    `an unrelated project's conversation became visible:\n${describeList(list)}`,
  ).toBe(false);

  const r = revoke(PROJECT_A, SHARED);
  expect(r.status, `revoke failed:\n${r.stdout}${r.stderr}`).toBe("Success");
});
