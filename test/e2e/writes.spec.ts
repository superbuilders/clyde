import { test, expect } from "@playwright/test";
import { ssm } from "./ssm";

// Writes into a user's tree, from the web UI.
//
// Both features here were broken in multi-user mode and fine in solo mode,
// which is why they survived to a deployed box: the service runs as root with
// CAP_DAC_READ_SEARCH but deliberately WITHOUT CAP_DAC_OVERRIDE, so a direct
// os.Create or os.Remove inside a user's 0750 tree fails with EACCES. Measured
// on the box before the fix:
//
//   # capsh --drop=cap_dac_override -- -c 'touch .../scratch/x'
//   touch: cannot touch '.../scratch/x': Permission denied
//
// The fix routes both through a child process holding the user's credential
// (principal.WriteFrom / principal.RemoveAs), the same way MkdirAs already
// did. So these tests assert two different things, and the second matters as
// much as the first:
//
//   1. the operation succeeds, and
//   2. the resulting file is owned by the USER, not by root.
//
// A root-owned file in a user's project would pass a naive "did it work?"
// check and then fail later when the agent — which runs as that user — tried
// to read or rewrite it. That delayed failure is the thing worth preventing.

const HOME_RE = /^\/srv\/bonnie\/users\/[a-z0-9._-]+$/;

/** The project directory the authenticated user owns, from the API. */
async function ownedProject(
  page: import("@playwright/test").Page,
): Promise<string> {
  const projects: Array<{ path: string }> = await page.evaluate(() =>
    fetch("/api/projects")
      .then((r) => r.json())
      .then((b) => (Array.isArray(b) ? b : (b.projects ?? []))),
  );
  expect(projects.length, "the user has at least one project").toBeGreaterThan(
    0,
  );
  const scratch =
    projects.find((p) => p.path.endsWith("/code/scratch")) ?? projects[0];
  return scratch.path;
}

test("uploading a file writes it into the project, owned by the user", async ({
  page,
}) => {
  await page.goto("/");
  await page.waitForLoadState("networkidle");

  const cwd = await ownedProject(page);
  console.log("[writes] project:", cwd);
  // Guard the guard: if this were not under a user home, "owned by the user"
  // would be trivially true and the test would prove nothing.
  expect(cwd).toContain("/srv/bonnie/users/");

  const name = `e2e-upload-${Date.now()}.txt`;
  const body = "bonnie e2e upload payload";

  // Through the browser's own fetch, carrying the session cookie, exactly as
  // the UI does it. Driving the file chooser would also exercise the picker,
  // but the picker was never the broken part and it makes the failure harder
  // to localise.
  const res = await page.evaluate(
    async ([cwd, name, body]) => {
      const fd = new FormData();
      fd.append("cwd", cwd!);
      fd.append("file", new File([body!], name!, { type: "text/plain" }));
      const r = await fetch("/api/upload", { method: "POST", body: fd });
      return { status: r.status, text: await r.text() };
    },
    [cwd, name, body],
  );
  console.log("[writes] upload response:", res.status, res.text);

  // The pre-fix failure was a 500 carrying "permission denied". Assert on the
  // status AND surface the body, so a regression reads as a cause rather than
  // as a bare number.
  expect(res.status, `upload failed: ${res.text}`).toBe(200);
  expect(res.text).toContain(name);

  // Now the half that a 200 does not prove.
  const stat = ssm(`stat -c '%U %a %s' '${cwd}/${name}' 2>&1 || echo MISSING`);
  console.log("[writes] stat:", stat.stdout.trim());
  expect(stat.stdout, "uploaded file is on disk").not.toContain("MISSING");

  const [owner, , size] = stat.stdout.trim().split(/\s+/);
  expect(owner, "uploaded file must be owned by the user, not root").not.toBe(
    "root",
  );
  expect(Number(size), "uploaded bytes landed intact").toBe(body.length);

  // The user's agent must be able to rewrite what the UI dropped in, which is
  // the whole point of it not being root-owned.
  const rw = ssm(
    `sudo -u '${owner}' sh -c "echo appended >> '${cwd}/${name}'" 2>&1 && echo WRITABLE || echo NOT_WRITABLE`,
  );
  expect(
    rw.stdout,
    "the owning user can write to their own uploaded file",
  ).toContain("WRITABLE");

  ssm(`rm -f '${cwd}/${name}'`);
});

test("deleting a message removes the file from a stopped session", async ({
  page,
}) => {
  await page.goto("/");
  await page.waitForLoadState("networkidle");

  const cwd = await ownedProject(page);
  const owner = ssm(`stat -c '%U' '${cwd}'`).stdout.trim();
  expect(owner, "project is owned by a real user").toMatch(/^[a-z]/);
  expect(owner).not.toBe("root");

  // Seed a stopped session AS THE USER, so the fixture matches what the agent
  // would have produced. Seeding it as root would create a tree the service
  // could not delete from for reasons unrelated to the bug under test, and the
  // test would then pass or fail for the wrong reason.
  const sid = `e2e-del-${Date.now()}`;
  const dir = `${cwd}/.clyde/sessions/${sid}`;
  const filename = "2026-01-01T00-00-00.000_user.md";
  const seed = ssm(
    [
      `sudo -u '${owner}' mkdir -p '${dir}'`,
      `sudo -u '${owner}' sh -c "printf 'seeded by e2e\\n' > '${dir}/${filename}'"`,
      `sudo -u '${owner}' sh -c "printf 'keep me\\n' > '${dir}/2026-01-01T00-00-01.000_assistant.md'"`,
      `stat -c '%U' '${dir}/${filename}'`,
    ].join(" && "),
  );
  console.log("[writes] seeded session owner:", seed.stdout.trim());
  expect(seed.status, `seeding failed: ${seed.stderr}`).toBe("Success");

  const res = await page.evaluate(
    async ([sid, filename, cwd]) => {
      const r = await fetch(
        `/api/sessions/${encodeURIComponent(sid!)}/messages/${encodeURIComponent(filename!)}?cwd=${encodeURIComponent(cwd!)}`,
        { method: "DELETE" },
      );
      return { status: r.status, text: await r.text() };
    },
    [sid, filename, cwd],
  );
  console.log("[writes] delete response:", res.status, res.text);
  expect(res.status, `delete failed: ${res.text}`).toBe(200);

  const after = ssm(
    `test -e '${dir}/${filename}' && echo STILL_THERE || echo GONE; ` +
      `test -e '${dir}/2026-01-01T00-00-01.000_assistant.md' && echo SIBLING_KEPT || echo SIBLING_LOST`,
  );
  console.log("[writes] after delete:", after.stdout.trim());
  expect(after.stdout, "the deleted message is gone from disk").toContain(
    "GONE",
  );
  // Deleting one message must not take the conversation with it.
  expect(after.stdout, "other messages survive").toContain("SIBLING_KEPT");

  ssm(`rm -rf '${dir}'`);
});

// Worktree create/delete had the same root-writes-into-user-tree defect, plus
// something worse: neither handler had ANY authorization check. That was
// masked because the write failed anyway — the kernel was standing in for
// application code. Running them as the user removes that accident, so the
// ownership check had to be added in the same change.

test("creating and deleting a worktree runs as the user, not as root", async ({
  page,
}) => {
  await page.goto("/");
  await page.waitForLoadState("networkidle");

  // parent_path is a CONTAINER of worktrees; the handler looks for a child
  // holding a .git and adds the new worktree as its sibling. The e2e user has
  // no git project, so seed one as the user (runuser), never as root.
  const home = (await ownedProject(page)).replace(/\/code\/.*$/, "");
  expect(home, "home looks right").toMatch(HOME_RE);
  const user = home.split("/").pop()!;
  const parent = `${home}/code/e2e-wt-${Date.now()}`;
  const branch = `feat-${Date.now()}`;

  const setup = ssm(
    `runuser -u ${user} -- bash -lc '` +
      `mkdir -p ${parent}/main && cd ${parent}/main && git init -q -b main && ` +
      `git -c user.email=e2e@x -c user.name=e2e commit -q --allow-empty -m seed && echo SEEDED'`,
  );
  console.log("[writes] seed:", setup.stdout, setup.stderr);
  expect(setup.stdout, "seed repo created").toContain("SEEDED");

  try {
    const created = await page.evaluate(
      async ([parent_path, branch_name]) => {
        const r = await fetch("/api/worktrees", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ parent_path, branch_name }),
        });
        return { status: r.status, text: await r.text() };
      },
      [parent, branch],
    );
    console.log("[writes] worktree create:", created.status, created.text);
    expect(created.status, `create failed: ${created.text}`).toBe(200);

    const wt = `${parent}/${branch}`;
    const probe = ssm(
      `if [ -d ${wt} ]; then echo "WT_PRESENT $(stat -c %U ${wt})"; else echo WT_ABSENT; fi; ` +
        `if [ -d ${wt}/.clyde/sessions ]; then echo "SESSIONS_PRESENT $(stat -c %U ${wt}/.clyde/sessions)"; else echo SESSIONS_ABSENT; fi`,
    ).stdout;
    console.log("[writes] worktree probe:", probe);
    expect(probe, "worktree directory exists").toContain("WT_PRESENT");
    expect(probe, ".clyde/sessions exists").toContain("SESSIONS_PRESENT");
    // Ownership is the point: a root-owned worktree passes a naive
    // "did it work?" check and breaks the agent on its first commit.
    expect(probe, "worktree owned by the user").toContain(`WT_PRESENT ${user}`);
    expect(probe, "sessions dir owned by the user").toContain(
      `SESSIONS_PRESENT ${user}`,
    );

    const deleted = await page.evaluate(
      async ([worktree_path, parent_path]) => {
        const r = await fetch("/api/worktrees/delete", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ worktree_path, parent_path }),
        });
        return { status: r.status, text: await r.text() };
      },
      [wt, parent],
    );
    console.log("[writes] worktree delete:", deleted.status, deleted.text);
    expect(deleted.status, `delete failed: ${deleted.text}`).toBe(200);
    const gone = ssm(`[ -d ${wt} ] && echo STILL_THERE || echo GONE`).stdout;
    expect(gone, "worktree removed from disk").toContain("GONE");
  } finally {
    ssm(`rm -rf ${parent}`);
  }
});

test("the viewer cache actually persists to disk", async () => {
  // saveCache discarded its error, so a cache that never once wrote looked
  // identical to a healthy one: sessions silently re-scanned from scratch on
  // every restart. The file's existence is the whole assertion.
  const probe = ssm(
    `f=/var/lib/bonnie/viewer-cache.json; ` +
      // A distinct marker, not the filename: "ls: cannot access '<file>'"
      // also contains the filename, so matching on that passes when the
      // file is absent — which is the exact bug under test.
      `if [ -s "$f" ]; then echo "CACHE_PRESENT $(stat -c '%U %a %s' "$f")"; else echo CACHE_ABSENT; fi; ` +
      `journalctl -u bonnie-web.service --since "-10 min" | grep -i "cache save failed" || true`,
  ).stdout;
  console.log("[writes] cache probe:", probe);
  expect(probe, "cache file exists and is non-empty").toContain(
    "CACHE_PRESENT",
  );
  expect(probe, "no save errors logged").not.toContain("cache save failed");
});
