import { test, expect } from "@playwright/test";
test("probe", async ({ page }) => {
  await page.goto("/");
  const pr = await page.request.get("/api/projects");
  console.log("PROJECTS:", await pr.text());
  const body = await pr.json();
  const list = body.projects || body || [];
  const cwd = (list[0]?.cwd || list[0]?.path);
  console.log("CWD:", cwd);
  const r = await page.request.post("/api/sessions/new", { data: { cwd } });
  console.log("NEW:", r.status(), await r.text());
});
