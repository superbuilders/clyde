import { test, expect } from "@playwright/test";

// PLAN.md §M3 — the isolation gate.
//
//   "Bob's agent cannot read Alice's tree."
//
// There are two independent halves and this file asserts both, because either
// one alone is a false sense of safety:
//
//   1. The kernel half. Agents run as real Unix users, so Bob's agent reading
//      Alice's home gets EACCES. This is a permission bit, not a policy check,
//      and it holds even if the web layer is wrong.
//   2. The application half. The webserver runs as root and can read
//      everything, so it must decide what to *show*. The session cache is
//      filled by one scanner with no principal — it sees every user's
//      sessions — so the listing is filtered per request.
//
// We authenticate as the e2e user (bonnie-e2e). "Alice" is the operator
// account, anthony-beckner, seeded with a marker file this test must never be
// able to see.
//
// SEED THE FIXTURE FIRST: scripts/bonnie-m3-fixture.sh. Against a rebuilt box
// with no Alice data, the "nothing leaked" assertions would pass vacuously,
// which is the worst way for a security test to be green. Bob cannot create
// or check the fixture himself — that is the milestone — so it is seeded out
// of band as root.

const ALICE_HOME = "/srv/bonnie/users/anthony-beckner";
// Bob is the authenticated test user; his own sessions must be visible.
const BOB_HOME = "/srv/bonnie/users/bonnie-e2e";
const ALICE_MARKER = "alice-private-marker-do-not-leak";

test("the session listing shows only the authenticated user's sessions", async ({ page }) => {
  const api = page.request;
  await page.goto("/");

  const res = await api.get("/api/sessions?days=0");
  expect(res.status(), "GET /api/sessions must be 200").toBe(200);
  const sessions: Array<{ id: string; cwd: string }> = (await res.json()).sessions ?? [];

  // days=0 means "no age filter", so this is every session the server is
  // willing to show us — the widest possible view.
  const leaked = sessions.filter((s) => (s.cwd || "").startsWith(ALICE_HOME));
  expect(
    leaked,
    `listing leaked ${leaked.length} of another user's sessions: ${leaked
      .map((s) => s.cwd)
      .join(", ")}`,
  ).toHaveLength(0);

  // The positive half, and the reason this file did not catch an empty
  // sidebar: "none of Alice's sessions leaked" is satisfied just as well by
  // seeing *nothing at all*. The background scanner spent M3 and M4 walking
  // the service account's home instead of the users', so every real sidebar
  // was empty and this assertion still passed. Isolation means seeing your
  // own sessions and not other people's; test both halves.
  const mine = sessions.filter((s) => (s.cwd || "").startsWith(BOB_HOME));
  expect(
    mine.length,
    `expected to see own sessions under ${BOB_HOME}, saw ${sessions.length} total`,
  ).toBeGreaterThan(0);

  console.log(
    `[m3] ${sessions.length} sessions visible, ${mine.length} mine, none under ${ALICE_HOME}`,
  );
});

test("the project listing shows only the authenticated user's projects", async ({ page }) => {
  const api = page.request;
  await page.goto("/");

  const res = await api.get("/api/projects");
  expect(res.status(), "GET /api/projects must be 200").toBe(200);
  const body = await res.json();
  const list: Array<{ cwd?: string; path?: string }> = body.projects || body || [];

  const leaked = list
    .map((p) => p.cwd || p.path || "")
    .filter((p) => p.startsWith(ALICE_HOME));
  expect(leaked, `project listing leaked: ${leaked.join(", ")}`).toHaveLength(0);

  // And we must still see our own, or the assertion above is vacuous: a server
  // that returns nothing at all would pass it.
  expect(list.length, "no projects visible at all — the filter is too strict").toBeGreaterThan(0);
  console.log(`[m3] ${list.length} projects visible, none under ${ALICE_HOME}`);
});

test("Bob's agent gets EACCES reading Alice's tree", async ({ page }) => {
  test.setTimeout(300_000);
  const api = page.request;
  await page.goto("/");

  // Start a session in our own project.
  const projRes = await api.get("/api/projects");
  const projects = await projRes.json();
  const list: Array<{ cwd?: string; path?: string }> = projects.projects || projects || [];
  expect(list.length, "no projects to start a session in").toBeGreaterThan(0);
  const cwd = (list[0].cwd || list[0].path)!;

  const newRes = await api.post("/api/sessions/new", { data: { cwd } });
  expect(newRes.status(), "POST /api/sessions/new must be 200").toBe(200);
  const sid = (await newRes.json()).session_id as string;
  expect(sid, "no session_id returned").toBeTruthy();
  console.log(`[m3] started session ${sid} in ${cwd}`);

  // Ask the agent to read Alice's tree. We want the raw shell result, so the
  // prompt asks it to report the error verbatim rather than work around it.
  const prompt =
    `Run exactly this command and report its complete output verbatim, ` +
    `including any error message. Do not try alternatives or work around a ` +
    `failure: cat ${ALICE_HOME}/code/scratch/.clyde/sessions/*/*.md`;

  await expect
    .poll(
      async () => {
        const r = await api.post(`/api/sessions/${sid}/messages`, {
          data: { cwd, content: prompt, force: true },
        });
        if (r.status() >= 200 && r.status() < 300) return true;
        const text = await r.text();
        // 409 = busy, "not running" = agent still booting. Both transient.
        if (r.status() === 409 || /not running/.test(text)) return false;
        expect(r.status(), `POST message failed: ${text}`).toBe(202);
        return false;
      },
      { timeout: 90_000, message: "agent never became ready to accept a message" },
    )
    .toBe(true);

  let transcript = "";
  await expect
    .poll(
      async () => {
        const r = await api.get(
          `/api/sessions/${sid}/messages?cwd=${encodeURIComponent(cwd)}`,
        );
        if (r.status() !== 200) return false;
        const msgs: Array<{ type?: string; content?: string }> = (await r.json()).messages || [];
        transcript = msgs.map((m) => `${m.type}: ${m.content}`).join("\n");
        return msgs.some(
          (m) => m.type === "assistant" && (m.content || "").trim().length > 0,
        );
      },
      { timeout: 240_000, message: "no assistant output ever streamed back" },
    )
    .toBe(true);

  // The assertion that matters: the marker never appears anywhere in the
  // transcript — not in tool output, not quoted back by the model.
  expect(
    transcript,
    "Alice's private content leaked into Bob's session transcript",
  ).not.toContain(ALICE_MARKER);

  // And the attempt must have actually been *denied*, not merely fruitless.
  // Without this, an agent that silently refused to run the command, or a
  // typo'd path, would pass the test above while proving nothing.
  expect(
    transcript.toLowerCase(),
    `expected a permission error in the transcript, got:\n${transcript.slice(0, 2000)}`,
  ).toMatch(/permission denied|eacces/);

  console.log(`[m3] agent was denied; ${transcript.length} chars of transcript`);
});
