import { test, expect } from "@playwright/test";
const ALICE_CWD = "/srv/bonnie/users/anthony-beckner/code/scratch";
const ALICE_SID = "2026-09-23T00-00-00_alice";

test("Bob cannot read Alice's transcript by guessing its id", async ({ page }) => {
  await page.goto("/");
  const r = await page.request.get(
    `/api/sessions/${encodeURIComponent(ALICE_SID)}/messages?cwd=${encodeURIComponent(ALICE_CWD)}`,
  );
  const body = await r.text();
  console.log("[leak] status:", r.status());
  console.log("[leak] body:", body.slice(0, 300));
  expect(r.status(), "reading another user's transcript must not be 200").not.toBe(200);
});
