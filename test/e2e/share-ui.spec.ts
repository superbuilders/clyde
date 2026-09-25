import { test, expect } from "@playwright/test";

// M4.2 — the Share button.
//
// sharing.spec.ts proves the grant/revoke API works. This proves a human can
// reach it. For the whole of M4.1 the backend was complete and there was no
// way to invoke it from the UI, which meant the milestone was not testable by
// the person it was built for; that gap is exactly what this file closes.
test("a project menu offers Share, and the modal lists real users", async ({ page }) => {
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

  // Open via the toolbar, not a project header menu: the header only exists
  // once a project has sessions, and sharing must not require having started
  // one. This is the path a user with a fresh checkout actually takes.
  const projects: Array<{ path: string; name: string }> = await page.evaluate(() =>
    fetch("/api/projects").then((r) => r.json()),
  );
  console.log("[share-ui] projects:", JSON.stringify(projects.map((p) => p.path)));
  expect(projects.length, "at least one project to share").toBeGreaterThan(0);

  const shareBtn = page.locator('button[title="Share a project"]');
  await shareBtn.waitFor({ state: "visible", timeout: 15_000 });
  await shareBtn.click();

  await expect(page.getByText("Share project")).toBeVisible({ timeout: 5_000 });

  // Pick the project, then assert the share controls appear.
  await page.selectOption("#share-project", projects[0].path);
  console.log("[share-ui] selected project:", projects[0].path);

  // The dropdown must be populated from the server, not hardcoded.
  const opts = await page.locator("#share-user option").allTextContents();
  console.log("[share-ui] dropdown options:", JSON.stringify(opts));
  for (const u of users) {
    expect(opts, `dropdown offers ${u}`).toContain(u);
  }

  // Read-only is the M4.2 contract and the UI must say so.
  await expect(page.getByText("read-only", { exact: false }).first()).toBeVisible();

  expect(errors, "no JS errors on the page").toEqual([]);
});
