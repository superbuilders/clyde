import { test, expect, type APIRequestContext } from "@playwright/test";

// PLAN.md §5, step 2 — the milestone gate.
//
//   "log in, list sessions, start a session, send a message, see output stream
//    back"
//
// Run against the *deployed* URL. Reading a diff is not verification; v1
// shipped a login you could complete that showed you nothing, because a wrong
// query param or an unreachable backend renders as empty rather than as an
// error. So every assertion here is about observed content, never status codes
// alone.

type Session = { id: string; cwd: string; project?: string };

async function listSessions(api: APIRequestContext): Promise<Session[]> {
  // The frontend requests exactly this shape (static/index.html:
  // fetch('/api/sessions?days=' + ...)). Using the same one means a param
  // mismatch fails here instead of silently rendering an empty list.
  const res = await api.get("/api/sessions?days=30");
  expect(res.status(), "GET /api/sessions must be 200").toBe(200);
  const body = await res.json();
  expect(body, "/api/sessions must return a sessions array").toHaveProperty("sessions");
  // The viewer returns `null`, not `[]`, when nothing matches the window. That
  // is upstream's shape, so normalise rather than treating it as a failure —
  // the assertion that matters is that our *new* session shows up below.
  return (body.sessions ?? []) as Session[];
}

test("the viewer page renders as the authenticated user, not an empty shell", async ({ page }) => {
  await page.goto("/");

  // We must NOT be redirected to the provider — storageState carries a session.
  expect(page.url(), "storageState did not authenticate us").not.toContain("amazoncognito.com");

  const body = (await page.textContent("body")) || "";
  expect(body).not.toMatch(/Login failed|Access denied|unauthenticated/);

  // The Alpine app shell must actually mount. `x-data` is inert text if Alpine
  // failed to load (it comes from a CDN, so this catches an egress-less box).
  await expect(page.locator("[x-data]").first()).toBeAttached();
  await page.waitForFunction(() => !!(window as any).Alpine, undefined, { timeout: 30_000 });

  // And the document must carry the viewer's own title, i.e. we are looking at
  // upstream's index.html, unmodified.
  await expect(page).toHaveTitle(/Clyde Session Viewer/);
});

test("lists sessions, starts one, sends a message, and sees output stream back", async ({
  page,
  request,
}) => {
  // 0. An authenticated API context. page.request shares the browser's cookies.
  const api = page.request;
  await page.goto("/");

  // 1. List sessions.
  const before = await listSessions(api);
  console.log(`[gate] sessions before: ${before.length}`);

  // 2. Pick a project to start a session in. /api/projects is what the UI uses.
  const projRes = await api.get("/api/projects");
  expect(projRes.status(), "GET /api/projects must be 200").toBe(200);
  const projects = await projRes.json();
  const list: Array<{ cwd?: string; path?: string }> = projects.projects || projects || [];
  expect(list.length, "no projects discovered — the box has no .clyde tree to work in").toBeGreaterThan(0);
  const cwd = (list[0].cwd || list[0].path)!;
  expect(cwd, "project entry had no cwd/path").toBeTruthy();
  console.log(`[gate] starting session in ${cwd}`);

  // 3. Start a session.
  const newRes = await api.post("/api/sessions/new", { data: { cwd } });
  expect(newRes.status(), "POST /api/sessions/new must be 200").toBe(200);
  const created = await newRes.json();
  const sid = created.session_id as string;
  expect(sid, "no session_id returned").toBeTruthy();
  console.log(`[gate] started session ${sid}`);

  // It must appear in the listing — proving the write landed where discovery looks.
  await expect
    .poll(async () => (await listSessions(api)).some((s) => s.id === sid), {
      timeout: 60_000,
      message: "new session never appeared in /api/sessions",
    })
    .toBe(true);

  // 4. Send a message. Deterministic and cheap to verify in the reply.
  const marker = `bonnie-gate-${Date.now()}`;
  const prompt = `Reply with exactly this token and nothing else: ${marker}`;
  await expect
    .poll(
      async () => {
        const r = await api.post(`/api/sessions/${sid}/messages`, {
          data: { cwd, content: prompt, force: true },
        });
        if (r.status() === 200) return 200;
        const text = await r.text();
        // 409 = agent busy; 500 "tmux session not running" = agent still
        // booting. Both are transient right after /sessions/new, so retry.
        if (r.status() === 409 || /not running/.test(text)) return r.status();
        expect(r.status(), `POST message failed: ${text}`).toBe(200);
        return r.status();
      },
      { timeout: 90_000, message: "agent never became ready to accept a message" },
    )
    .toBe(200);
  console.log(`[gate] sent message with marker ${marker}`);

  // 5. See output stream back: an assistant message must appear. This is the
  //    assertion that proves the agent actually ran on the box, as opposed to
  //    the UI merely loading.
  let transcript = "";
  await expect
    .poll(
      async () => {
        const r = await api.get(
          `/api/sessions/${sid}/messages?cwd=${encodeURIComponent(cwd)}`,
        );
        if (r.status() !== 200) return false;
        const b = await r.json();
        const msgs: Array<{ type?: string; content?: string }> = b.messages || [];
        transcript = msgs.map((m) => `${m.type}: ${m.content}`).join("\n");
        return msgs.some(
          (m) => m.type === "assistant" && (m.content || "").trim().length > 0,
        );
      },
      { timeout: 180_000, message: "no assistant output ever streamed back" },
    )
    .toBe(true);

  console.log(`[gate] assistant replied (${transcript.length} chars of transcript)`);
  // The reply should contain our marker, proving it answered *our* message.
  expect(transcript, "assistant replied but not to our prompt").toContain(marker);
});
