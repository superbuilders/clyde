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
// Two things this file is careful about, both learned the hard way:
//
//   1. Prove access exists BEFORE revoking it. "Bob cannot see it" passes
//      trivially against a share that never happened, a typo'd path, or a
//      grant that silently failed. A revoke assertion is only meaningful if
//      it follows an assertion that the thing was there a moment earlier.
//      This is the M3 vacuity lesson.
//
//   2. Revoke one of two OVERLAPPING shares. Both live under ~/code, so both
//      need a traverse bit on it. A revoke that strips the shared ancestor
//      breaks the surviving share silently — nothing errors, the other
//      project simply disappears. Reference-counting that ancestor by hand is
//      the bug this catches.

const ALICE = "anthony-beckner";
const BOB = "bonnie-e2e";
const ALICE_CODE = `/srv/bonnie/users/${ALICE}/code`;
const SHARE_A = `${ALICE_CODE}/shared-a`;
const SHARE_B = `${ALICE_CODE}/shared-b`;
const PRIVATE = `/srv/bonnie/users/${ALICE}`;

/** Bob's visible project directories, as the deployed viewer reports them. */
async function bobProjects(page: import("@playwright/test").Page): Promise<string[]> {
  // Rescan first: projects are derived from a session cache filled by a
  // background scanner, so a share granted a moment ago is not visible until
  // the tree is re-read.
  await page.request.post("/api/sessions/scan");
  const res = await page.request.get("/api/projects");
  expect(res.status(), "GET /api/projects must be 200").toBe(200);
  const body = await res.json();
  const list = Array.isArray(body) ? body : (body.projects ?? []);
  return list.map((p: { path?: string; cwd?: string }) => p.path ?? p.cwd ?? "");
}

test.describe.configure({ mode: "serial" });

test.afterAll(() => {
  // Leave the box in the state the fixture creates, whatever happened above.
  // A leftover grant would make the next run's opening assertion pass for the
  // wrong reason.
  bonnieShare(`--owner ${ALICE} --sharee ${BOB} --path ${SHARE_A} --revoke`);
  bonnieShare(`--owner ${ALICE} --sharee ${BOB} --path ${SHARE_B} --revoke`);
});

test("a share appears for the sharee, is read-only, and revoke takes it away", async ({ page }) => {
  // ── Before: Bob can see neither directory ────────────────────────────────
  const before = await bobProjects(page);
  expect(before, `${SHARE_A} was already visible — the fixture did not reset`).not.toContain(SHARE_A);

  // ── Alice grants ─────────────────────────────────────────────────────────
  const granted = bonnieShare(`--owner ${ALICE} --sharee ${BOB} --path ${SHARE_A}`);
  expect(granted.status, `bonnie share failed:\n${granted.stdout}${granted.stderr}`).toBe("Success");

  // ── Bob refreshes and it is there ────────────────────────────────────────
  // This assertion is what makes the revoke assertion below mean anything.
  const afterGrant = await bobProjects(page);
  expect(
    afterGrant,
    `the shared project never became visible to ${BOB}:\n${afterGrant.join("\n")}`,
  ).toContain(SHARE_A);

  // ── It is read-only ──────────────────────────────────────────────────────
  // A share must not become a place Bob can start an agent: he cannot write,
  // so the session would fail to record itself.
  const newSession = await page.request.post("/api/sessions/new", {
    data: { cwd: SHARE_A },
  });
  expect(
    newSession.status(),
    "a read-only share must refuse a new session rather than starting one that cannot write",
  ).toBe(403);

  // And the kernel agrees, independently of what the web layer decided.
  // Assert on an explicit outcome word rather than an exit code: a failed
  // redirect exits 2 under dash and 1 under bash, so a hardcoded code makes
  // this pass or fail on which shell the box happens to use.
  const write = ssm(
    `if runuser -u ${BOB} -- sh -c 'echo x > ${SHARE_A}/should-not-exist' 2>/dev/null; ` +
      `then echo OUTCOME=WROTE; else echo OUTCOME=DENIED; fi`,
  );
  expect(write.stdout, "Bob was able to write into a read-only share").toContain("OUTCOME=DENIED");

  // ── Sharing one directory shares only that directory ─────────────────────
  const stillHidden = await bobProjects(page);
  expect(stillHidden, "an unshared sibling became visible").not.toContain(SHARE_B);
  expect(stillHidden, "Alice's home became visible").not.toContain(PRIVATE);

  // ── Revoke, and the access is actually gone ──────────────────────────────
  const revoked = bonnieShare(`--owner ${ALICE} --sharee ${BOB} --path ${SHARE_A} --revoke`);
  expect(revoked.status, `revoke failed:\n${revoked.stdout}${revoked.stderr}`).toBe("Success");

  const afterRevoke = await bobProjects(page);
  expect(afterRevoke, "the project was still listed after revoke").not.toContain(SHARE_A);

  // Complete means the access is gone, not that the ACL entry is gone
  // (PLAN.md §8 item 4). Ask the kernel, as Bob, not the web layer.
  const read = ssm(
    `if runuser -u ${BOB} -- sh -c 'cat ${SHARE_A}/.clyde/sessions/*/*.md' >/dev/null 2>&1; ` +
      `then echo OUTCOME=READ; else echo OUTCOME=DENIED; fi`,
  );
  expect(
    read.stdout,
    "Bob could still read the revoked share — revocation removed the ACL but not the access",
  ).toContain("OUTCOME=DENIED");
});

test("revoking one share leaves an overlapping one working", async ({ page }) => {
  // Both directories sit under ~/code, so both grants depend on the same
  // traverse bit. This is the case that breaks silently.
  const a = bonnieShare(`--owner ${ALICE} --sharee ${BOB} --path ${SHARE_A}`);
  expect(a.status, `granting A failed:\n${a.stdout}${a.stderr}`).toBe("Success");
  const b = bonnieShare(`--owner ${ALICE} --sharee ${BOB} --path ${SHARE_B}`);
  expect(b.status, `granting B failed:\n${b.stdout}${b.stderr}`).toBe("Success");

  const both = await bobProjects(page);
  expect(both, "share A never appeared").toContain(SHARE_A);
  expect(both, "share B never appeared").toContain(SHARE_B);

  // Revoke only A. B must survive.
  const revoked = bonnieShare(`--owner ${ALICE} --sharee ${BOB} --path ${SHARE_A} --revoke`);
  expect(revoked.status, `revoke failed:\n${revoked.stdout}${revoked.stderr}`).toBe("Success");

  const after = await bobProjects(page);
  expect(after, "the revoked share is still visible").not.toContain(SHARE_A);
  expect(
    after,
    "revoking one share broke an unrelated one: the corridor they share was stripped",
  ).toContain(SHARE_B);

  // And B is genuinely readable, not merely listed.
  const read = ssm(
    `if runuser -u ${BOB} -- sh -c 'ls ${SHARE_B} >/dev/null'; then echo OUTCOME=READ; else echo OUTCOME=DENIED; fi`,
  );
  expect(read.stdout, "the surviving share is listed but no longer readable").toContain("OUTCOME=READ");
});
