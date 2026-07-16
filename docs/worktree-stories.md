# Worktree Support — User Stories

**Design doc:** [docs/worktree-support.md](./worktree-support.md)
**Branch:** `worktree-support`

Stories are ordered by dependency. Stories within the same tier can be done in parallel.

---

## Tier 1: Backend Foundation (no frontend changes)

### WT-1: TUI `findSessionsRoot()` — respect existing `.clyde/sessions/` in cwd

**As** a user running Clyde from a non-git directory that has a `.clyde/sessions/` folder,
**I want** Clyde to use that local `.clyde/sessions/` for session storage,
**so that** sessions created by the session viewer in a worktree parent folder can be resumed with `clyde -r`.

**Changes:**
- `agent/session/session.go` → `findSessionsRoot()`

**Acceptance Criteria:**
1. When cwd is a git repo, behavior is unchanged — `git rev-parse --show-toplevel` determines the session root.
2. When cwd is NOT a git repo but `<cwd>/.clyde/sessions/` exists on disk, `findSessionsRoot()` returns `<cwd>/.clyde/sessions/`.
3. When cwd is NOT a git repo and `<cwd>/.clyde/sessions/` does NOT exist, fallback to `~/.clyde/sessions/` (existing behavior).
4. `clyde -r <session-id>` works correctly when invoked from a non-git directory that has `.clyde/sessions/` containing that session.
5. Existing tests continue to pass.
6. New unit test: `findSessionsRoot()` in a temp directory with a pre-existing `.clyde/sessions/` returns that path.
7. New unit test: `findSessionsRoot()` in a temp directory WITHOUT `.clyde/sessions/` returns `~/.clyde/sessions/`.

---

### WT-2: Session Viewer Backend — worktree detection helpers

**As** the session viewer backend,
**I want** helper functions that detect whether a project directory is part of a git worktree group and identify all sibling worktrees,
**so that** the scan and API layers can annotate sessions with worktree grouping info.

**Changes:**
- `session-viewer/main.go` — new functions, new types

**Acceptance Criteria:**
1. New function `detectWorktreeGroup(dir string) *WorktreeGroup` that:
   - Returns `nil` for non-git directories.
   - Returns `nil` for normal git repos with no linked worktrees.
   - Returns `nil` for worktree groups where worktrees are NOT siblings (different parent dirs).
   - Returns a `WorktreeGroup` with `ParentDir`, `ParentName`, and `Worktrees []WorktreeEntry` for sibling worktree groups.
2. New function `parseWorktreeList(output string) []WorktreeEntry` that parses `git worktree list --porcelain` output into structs with `Path`, `Branch`, and `Name` fields.
3. New types: `WorktreeGroup` and `WorktreeEntry` as specified in the design doc.
4. Detection works from both the main worktree and any linked worktree — both return the same `WorktreeGroup`.
5. The `allSiblingsOf` check compares `filepath.Dir(path)` for all worktree entries; all must share the same parent directory.
6. Unit test: parse a sample `git worktree list --porcelain` output with 3 worktrees.
7. Integration test: create a temp git repo with `git worktree add`, run `detectWorktreeGroup` from both the main and linked worktree, verify identical results.
8. Integration test: create worktrees in non-sibling locations, verify `detectWorktreeGroup` returns `nil`.

---

### WT-3: Session Viewer Backend — annotate sessions with worktree group info during scan

**As** the session viewer,
**I want** `backgroundScan()` to detect worktree groups and annotate cached sessions with their parent folder info,
**so that** the API can return worktree grouping data to the frontend.

**Changes:**
- `session-viewer/main.go` → `backgroundScan()`, `CachedSession`, `SessionResponse`, `getSessions()`

**Acceptance Criteria:**
1. `CachedSession` has new fields: `WorktreeParent string` and `WorktreeParentName string` (both `json:"...,omitempty"`).
2. `SessionResponse` has matching new fields.
3. During `backgroundScan()`, after discovering project directories, `detectWorktreeGroup()` (from WT-2) is called for each. If a group is detected:
   - All sessions from worktree member directories get `WorktreeParent` set to the parent folder path and `WorktreeParentName` set to its basename.
   - Sessions from the parent folder itself (if it has `.clyde/sessions/`) also get these fields set.
4. `getSessions()` populates the new fields on `SessionResponse` from the cache.
5. Non-worktree projects have empty `WorktreeParent`/`WorktreeParentName` (omitted from JSON).
6. `GET /api/sessions` response includes `worktree_parent` and `worktree_parent_name` for worktree sessions.
7. Scan performance: `git worktree list` is called at most once per unique git repo (deduplicate across worktrees that share the same `--git-common-dir`).

**Depends on:** WT-2

---

### WT-4: Session Viewer Backend — discover all sibling worktrees from any member

**As** the session viewer,
**I want** discovering one worktree to automatically discover all of its siblings,
**so that** worktrees without `.clyde/` directories still appear in the sidebar (with 0 sessions) and can have sessions created in them.

**Changes:**
- `session-viewer/main.go` → `backgroundScan()`, `discoverProjectDirs()` integration

**Acceptance Criteria:**
1. When a worktree group is detected during scan, ALL worktree paths from `git worktree list` are added to the set of discovered project directories.
2. Worktrees without `.clyde/sessions/` directories appear in the API response (with `message_count: 0`).
3. The parent folder is also added to the discovery set.
4. Creating a session (via the existing `POST /api/sessions/new` endpoint) in a previously-undiscovered worktree works — the `.clyde/sessions/` directory is created on demand.
5. Scan does not run `git worktree list` redundantly — if worktree A discovers siblings B and C, processing B and C does not re-run detection.

**Depends on:** WT-2, WT-3

---

### WT-5: Session Viewer Backend — `POST /api/worktrees` endpoint

**As** a user of the session viewer,
**I want** to create a new git worktree (and branch) from the session viewer UI,
**so that** I can start a new branch+worktree without leaving the viewer.

**Changes:**
- `session-viewer/main.go` — new endpoint, route registration

**Acceptance Criteria:**
1. `POST /api/worktrees` accepts `{ "parent_path": "<path>", "branch_name": "<name>" }`.
2. The endpoint finds an existing git worktree inside `parent_path` to run git commands against (searches child directories for a `.git` file or directory).
3. Runs `git -C <child_git_dir> worktree add -b <branch_name> <parent_path>/<branch_name>`.
4. Creates `<parent_path>/<branch_name>/.clyde/sessions/` directory (with `0755` permissions).
5. Triggers a background rescan after creation.
6. Returns `200 { "status": "ok", "path": "<new_worktree_path>", "branch": "<branch_name>" }` on success.
7. Returns `400` with descriptive error if:
   - `parent_path` is empty or `branch_name` is empty.
   - No git worktree is found in `parent_path`.
   - `branch_name` contains invalid characters (spaces, `..`, etc.).
8. Returns `409` or `500` with the git error message if:
   - Branch already exists.
   - Target directory already exists.
   - Git command fails for any other reason.
9. After creation, the new worktree appears in subsequent `GET /api/sessions` responses with its `worktree_parent` fields populated.
10. Route registered: `api.POST("/worktrees", createWorktree)`.

**Depends on:** WT-2

---

### WT-6: Session Viewer Backend — `GET /api/projects` includes worktree info

**As** the session viewer frontend,
**I want** `GET /api/projects` to return worktree relationship data,
**so that** I can render the "+" dropdown correctly for worktree groups and populate the worktree creation modal.

**Changes:**
- `session-viewer/main.go` → `ProjectInfo` type, `getProjects()`

**Acceptance Criteria:**
1. `ProjectInfo` has new fields: `ParentPath string`, `ParentName string`, `IsWorktreeGroup bool`, `Worktrees []WorktreeEntry` (all `omitempty`).
2. Projects that are part of a worktree group have `ParentPath` and `ParentName` set.
3. At least one entry per worktree group has `IsWorktreeGroup: true` and `Worktrees` populated with all members. (This can be a synthetic entry for the parent folder, or on the first discovered member.)
4. Non-worktree projects have no worktree fields (omitted from JSON).
5. The parent folder itself is included as a project entry if it has `.clyde/sessions/`.

**Depends on:** WT-2

---

## Tier 2: Frontend (depends on Tier 1 backend)

### WT-7: Session Viewer Frontend — two-level sidebar for worktree groups

**As** a user with a worktree-based project,
**I want** the session viewer sidebar to group my worktrees under their shared parent folder,
**so that** I can see all worktrees for a project together and navigate between them.

**Changes:**
- `session-viewer/static/index.html` → `groupedSessions`, sidebar template

**Acceptance Criteria:**
1. Sessions with `worktree_parent_name` set are grouped under a parent header (named by `worktree_parent_name`).
2. Within the parent group, sessions are sub-grouped by their project name (the worktree's basename), each with its own collapsible sub-header showing the branch name.
3. Parent-level sessions (where `cwd === worktree_parent`) appear directly under the parent header, above the worktree sub-groups.
4. Non-worktree projects render exactly as they do today — flat list of sessions under a project header. Zero visual or behavioral regression.
5. Both the parent group header and worktree sub-headers are independently collapsible.
6. The collapse state for worktree sub-headers is persisted in preferences (same mechanism as existing project collapse).
7. Session count badge on the parent header shows total sessions across all children and parent-level sessions.
8. Worktrees with 0 sessions still appear as sub-headers (so the user can "+" a session into them).
9. Search/filter works across worktree groups — searching for a branch name surfaces the relevant worktree's sessions.
10. The "Live" toggle filters within worktree groups correctly.

**Depends on:** WT-3, WT-4, WT-6

---

### WT-8: Session Viewer Frontend — "+" button dropdown for worktree groups

**As** a user viewing a worktree group in the sidebar,
**I want** the "+" button on the parent header to offer "New Session" and "New Worktree" options,
**so that** I can either start a session in the parent folder or create an entirely new worktree.

**Changes:**
- `session-viewer/static/index.html` → "+" button template, dropdown component

**Acceptance Criteria:**
1. On a **non-worktree project header**, the "+" button behaves exactly as today — single click creates a session. No dropdown.
2. On a **worktree group parent header**, clicking "+" shows a small dropdown with two options:
   - **"New Session"** — creates a session in the parent folder's CWD (calls existing `POST /api/sessions/new` with `cwd` = parent path).
   - **"New Worktree"** — opens the worktree creation modal (WT-9).
3. On a **worktree child sub-header**, the "+" button behaves as a single click — creates a session in that worktree's CWD.
4. The dropdown closes when clicking outside it or selecting an option.
5. The dropdown is visually consistent with the existing UI (DaisyUI/Tailwind styling).

**Depends on:** WT-7

---

### WT-9: Session Viewer Frontend — worktree creation modal

**As** a user who wants to start work on a new feature branch,
**I want** a modal dialog where I can type a branch name and create a new worktree,
**so that** I can create worktrees from the viewer without using the terminal.

**Changes:**
- `session-viewer/static/index.html` — new modal component, API call

**Acceptance Criteria:**
1. Modal appears when "New Worktree" is selected from the "+" dropdown (WT-8).
2. Modal contains:
   - Title: "New Worktree"
   - Input field: "Branch name" (text input, autofocused)
   - Path preview: shows `<parent_path>/<branch_name>` updating live as user types
   - "Cancel" button: closes modal, no action
   - "Create" button: calls `POST /api/worktrees` with the parent path and branch name
3. "Create" button is disabled when the input is empty.
4. On success:
   - Modal closes.
   - A rescan is triggered (or the frontend polls until the new worktree appears).
   - The new worktree appears in the sidebar under the parent group with 0 sessions.
5. On error:
   - The error message from the API is displayed in the modal (below the input field, red text).
   - The modal stays open so the user can correct the input.
6. Enter key in the input field submits (same as clicking "Create").
7. Escape key closes the modal (same as clicking "Cancel").
8. Modal is styled consistent with the existing UI.

**Depends on:** WT-5, WT-8

---

## Story Dependency Graph

```
WT-1 (TUI findSessionsRoot)          ← independent, can start immediately

WT-2 (detection helpers)              ← independent, can start immediately
  ├── WT-3 (scan annotation)
  │     └── WT-7 (frontend sidebar)
  │           └── WT-8 (+ dropdown)
  │                 └── WT-9 (worktree modal)
  ├── WT-4 (sibling discovery)
  │     └── WT-7
  ├── WT-5 (POST /api/worktrees)
  │     └── WT-9
  └── WT-6 (GET /api/projects)
        └── WT-7
```

**Parallelism:** WT-1 and WT-2 can start simultaneously. WT-3, WT-4, WT-5, WT-6 can all start once WT-2 is done. WT-7 needs WT-3 + WT-4 + WT-6. WT-8 needs WT-7. WT-9 needs WT-5 + WT-8.
