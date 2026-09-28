import { test, expect } from "@playwright/test";

// M4.2 — the Share button.
//
// sharing.spec.ts proves the grant/revoke API works. This proves a human can
// reach it. For the whole of M4.1 the backend was complete and there was no
// way to invoke it from the UI, which meant the milestone was not testable by
// the person it was built for; that gap is exactly what this file closes.
//
// It now opens sharing from a conversation's own ⋮ menu. There is deliberately
// no other entry point: the toolbar button and its project picker were removed
// when the unit of sharing became the conversation, because a picker implies
// you choose what to share, and you do not — you share the thing you are
// looking at. A test that still reached for the toolbar would be testing a
// door we bricked up.
test("a conversation menu offers Share, and the modal lists real users", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  page.on("console", (m) => {
    // Alpine warns about unrelated upstream markup on some pages; only fail on
    // real errors, and report them verbatim so a failure is diagnosable.
    if (m.type() === "error") errors.push(m.text());
  });

  await page.goto("/");
  await page.waitForLoadState("networkidle");

  // The feature is gated on multi-user mode, so assert we are actually in it.
  // Otherwise a green test could just mean "the button is correctly hidden".
  const users: string[] = await page.evaluate(() =>
    fetch("/api/shareable-users").then((r) => r.json()),
  );
  console.log("[share-ui] shareable users:", JSON.stringify(users));
  expect(users.length, "multi-user mode with at least one other user").toBeGreaterThan(0);

  // A conversation to share. Unlike a project, this cannot be conjured by the
  // fixture alone being present — the sidebar has to have rendered it.
  //
  // /api/sessions answers with an envelope, not a bare array: the list rides
  // alongside preferences and the last scan time. Indexing into it as an array
  // yields undefined rather than an error, so a test that forgets this fails
  // with "received value must be a number", several lines from the cause.
  const sessions: Array<{ id: string; cwd: string }> = await page.evaluate(() =>
    fetch("/api/sessions?days=0")
      .then((r) => r.json())
      .then((b) => (Array.isArray(b) ? b : (b.sessions ?? []))),
  );
  console.log("[share-ui] sessions:", sessions.length);
  expect(sessions.length, "at least one conversation to share").toBeGreaterThan(0);

  // The ⋮ button only materialises on hover, so hover the row first. Scoped to
  // the first session card rather than the first ⋮ on the page, because
  // project headers have one too and clicking that would open the wrong menu
  // and still find a Share entry if we ever regressed.
  const card = page.locator(".group\\/card").first();
  await card.waitFor({ state: "visible", timeout: 15_000 });
  await card.hover();
  await card.locator("button", { hasText: "⋮" }).first().click();

  const shareItem = page.getByRole("button", { name: /Share…/ }).first();
  await shareItem.waitFor({ state: "visible", timeout: 5_000 });
  await shareItem.click();

  // "Share conversation", not "Share project": the title is the one place the
  // UI states what the unit is, and getting it wrong is how a user ends up
  // believing they gave away more or less than they did.
  await expect(page.getByText("Share conversation")).toBeVisible({ timeout: 5_000 });

  // The dropdown must be populated from the server, not hardcoded.
  const opts = await page.locator("#share-user option").allTextContents();
  console.log("[share-ui] dropdown options:", JSON.stringify(opts));
  for (const u of users) {
    expect(opts, `dropdown offers ${u}`).toContain(u);
  }

  // Read-only is the M4.2 contract and the UI must say so — and now it must
  // also say that the project is not included, which is the part a user would
  // otherwise have to take on faith.
  await expect(page.getByText("Read-only, and only this conversation", { exact: false })).toBeVisible();

  // The retired entry point must be gone, not merely unused. Leaving it would
  // mean two code paths to the same feature, one of which sends a body the
  // server no longer understands.
  await expect(page.locator('button[title="Share a project"]')).toHaveCount(0);

  expect(errors, "no JS errors on the page").toEqual([]);
});
