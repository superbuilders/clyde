import { test, expect } from "@playwright/test";

// PLAN.md §5 — headless login through the *real* hosted UI as a Cognito-native
// test user.
//
// Why a native user and not Google: Google actively blocks automated sign-in.
// The federated path converges on the same callback, the same token
// verification and the same cookie, so driving the native user exercises
// everything except Google's own form. The Google path is verified manually by
// AJ, once per milestone.
//
// There is deliberately no bypass to make this easier: no --dev-accept-token,
// no header auth, no impersonation. This test performs a real OIDC code flow.

const USERNAME = process.env.BONNIE_E2E_USERNAME;
const PASSWORD = process.env.BONNIE_E2E_PASSWORD;

test("logs in through the hosted UI and reaches the viewer", async ({ page }) => {
  expect(
    USERNAME,
    "BONNIE_E2E_USERNAME must be set (see `make e2e`, which reads Secrets Manager)",
  ).toBeTruthy();
  expect(PASSWORD, "BONNIE_E2E_PASSWORD must be set").toBeTruthy();

  // 1. Hitting the app unauthenticated must bounce us to the provider.
  await page.goto("/");
  await page.waitForURL(/amazoncognito\.com/, { timeout: 30_000 });

  // 2. The hosted UI's native email/password form. Cognito ships two forms in
  //    the same document (a modal and a non-modal copy), so scope to the
  //    *visible* inputs — `.first()` picks the hidden one.
  const username = page.locator('input[name="username"]:visible').first();
  const password = page.locator('input[name="password"]:visible').first();
  await username.waitFor({ state: "visible", timeout: 30_000 });
  await username.fill(USERNAME!);
  await password.fill(PASSWORD!);
  await page.locator('input[type="submit"]:visible, button[type="submit"]:visible').first().click();

  // 3. Back on our origin, authenticated. If the callback failed we render a
  //    visible "Login failed" page rather than a blank one — assert we did not
  //    land there.
  await page.waitForURL((u) => !u.hostname.includes("amazoncognito.com"), {
    timeout: 45_000,
  });
  await expect(page.locator("h1")).not.toHaveText(/Login failed|Access denied/, {
    timeout: 5_000,
  }).catch(() => {
    /* no h1 on the viewer page at all, which is the success case */
  });

  const body = await page.textContent("body");
  expect(body, "landed on an auth error page").not.toMatch(/Login failed|Access denied/);

  // 4. The session cookie must exist and be HttpOnly.
  const cookies = await page.context().cookies();
  const session = cookies.find((c) => c.name === "bonnie_session");
  expect(session, "bonnie_session cookie was not set").toBeTruthy();
  expect(session!.httpOnly, "session cookie must be HttpOnly").toBe(true);

  // 5. And the gated API must now answer as us, not 401.
  const res = await page.request.get("/api/sessions?days=30");
  expect(res.status(), "authenticated /api/sessions must not be 401").toBe(200);

  await page.context().storageState({ path: "state.json" });
});
