# Design: Git Worktree Support for Clyde (v3)

**Date:** July 2026
**Status:** Accepted
**Branch:** `worktree-support`

---

## Core Principle

**Worktrees are projects. The parent folder is just a grouping container.** The TUI's existing behavior — each worktree gets its own `.clyde/sessions/` — is already correct. The only real work is in the session viewer's sidebar: detect sibling worktrees, group them under their shared parent folder, and provide UI to create new worktrees.

## Mental Model

Given the recommended layout:
```
~/code/myproject/                ← parent folder (plain unix dir, not a git repo)
  myproject/                     ← main worktree (branch: main)
    .clyde/sessions/
  feature-x/                     ← linked worktree (branch: feature-x)
    .clyde/sessions/
```

The sidebar renders:
```
▼ myproject                      [+]  ← parent group header; [+] → "Session" | "Worktree"
    (parent-level sessions, if any)
  ▼ main (myproject)             [+]  ← main worktree; [+] → creates session
      session-1
      session-2
  ▼ feature-x                   [+]  ← linked worktree; [+] → creates session
      session-3
```

Non-worktree projects render exactly as they do today:
```
▼ other-project                  [+]  ← normal project; [+] → creates session
    session-4
    session-5
```

## Git Worktree Mechanics (Reference)

Confirmed by testing:

| Command | Main Worktree | Linked Worktree |
|---------|--------------|-----------------|
| `--show-toplevel` | `/path/to/main-repo` | `/path/to/feature-wt` |
| `--git-common-dir` | `.git` (relative) | `/path/to/main-repo/.git` (absolute) |
| `--git-dir` | `.git` | `/path/to/main-repo/.git/worktrees/feature-wt` |

Key insight: **`--git-common-dir` always points to the same `.git` directory** regardless of which worktree you're in. The parent of this directory is the main worktree root.

For non-worktree repos, `--git-common-dir` returns `.git` and its parent is the repo root — **identical to current behavior**.

Detection: a directory is a **linked worktree** if and only if `git rev-parse --git-dir` ≠ `git rev-parse --git-common-dir`.

The parent folder of all worktrees (the grouping container) is `dirname(<main-worktree-root>)`, which equals `dirname(<any-linked-worktree-root>)` when all worktrees are siblings.

`git worktree list --porcelain` returns all worktrees with paths and branches, callable from any worktree:
```
worktree /path/to/main
HEAD abc123
branch refs/heads/main

worktree /path/to/feature-x
HEAD def456
branch refs/heads/feature-x
```

`git worktree add -b <branch> <path>` creates both a new branch and a new worktree in one command.

## What Doesn't Change

- **TUI session storage**: `findSessionsRoot()` uses `--show-toplevel` → `<worktree>/.clyde/sessions/`. Each worktree is its own project. No change.
- **`--resume`/`-r`**: Looks in the current worktree's `.clyde/sessions/`. You can only resume sessions from the worktree you're in. Correct behavior. No change.
- **`--sessions`**: Lists sessions for the current worktree. No change.
- **Session file format**: No metadata files, no schema changes.
- **`.gitignore` handling**: Unchanged.
- **Prompt**: Shows the worktree's branch. Unchanged.
- **CWD model**: CWD is always the worktree root for worktree sessions, project root for normal projects. Unchanged.

## What Changes

### 1. TUI: One small `findSessionsRoot()` tweak

**Problem:** The parent folder is not a git repo. When the SV creates a session there and runs `cd <parent> && clyde -r <session>`, the TUI's `findSessionsRoot()` can't find the sessions because `git rev-parse --show-toplevel` fails, so it falls back to `~/.clyde/sessions/`.

**Fix:** Add a check between the git check and the home-dir fallback:

```
1. git rev-parse --show-toplevel → use <repo>/.clyde/sessions/ if found
2. NEW: if cwd/.clyde/sessions/ exists on disk → use it
3. Fallback to ~/.clyde/sessions/
```

Step 2 handles the parent-folder case: the SV already created `.clyde/sessions/` there, so the TUI finds it. This is also a general improvement — any non-git directory with a pre-existing `.clyde/sessions/` Just Works.

This is the **only TUI change**.

### 2. Session Viewer Backend: Worktree detection during scan

**When:** During `backgroundScan()`, after discovering project directories.

**Algorithm for each discovered project dir:**

```go
func detectWorktreeInfo(dir string) *WorktreeGroup {
    // Step 1: Is this inside a git repo?
    gitDir := exec("git -C <dir> rev-parse --git-dir")
    if err → return nil  // not a git repo at all

    commonDir := exec("git -C <dir> rev-parse --git-common-dir")

    // Step 2: Is this a linked worktree?
    // (linked worktrees have gitDir != commonDir)
    isLinkedWorktree := (gitDir != commonDir)

    if isLinkedWorktree {
        // The parent of commonDir is the main worktree root
        mainWorktreeRoot := parent(resolve(commonDir))
        // The parent of THAT is the container folder
        parentFolder := parent(mainWorktreeRoot)

        // Get all siblings
        worktrees := parseWorktreeList(exec("git -C <dir> worktree list --porcelain"))

        // Check: are all worktrees siblings (same parent dir)?
        if allSiblingsOf(parentFolder, worktrees) {
            return &WorktreeGroup{
                ParentDir: parentFolder,
                Worktrees: worktrees,
            }
        }
        // Non-sibling layout: don't group (treat as separate projects)
        return nil
    }

    // Step 3: This is a normal repo or main worktree.
    // Check if it has linked worktrees.
    worktrees := parseWorktreeList(exec("git -C <dir> worktree list --porcelain"))
    if len(worktrees) <= 1 {
        return nil  // no worktrees, normal repo
    }

    // Has worktrees. Check if they're siblings.
    parentFolder := parent(dir)  // dir is the main worktree
    if allSiblingsOf(parentFolder, worktrees) {
        return &WorktreeGroup{
            ParentDir: parentFolder,
            Worktrees: worktrees,
        }
    }
    return nil
}
```

**Key rule: only group if all worktrees are siblings.** If someone has worktrees scattered across the filesystem, don't try to group them — just treat each as a separate project. The sibling check is `dirname(path)` is the same for all worktree paths.

**Performance:** `git worktree list` runs once per discovered project dir during scan (every 60s). Typically <20 projects. Negligible.

### 3. Session Viewer Backend: New types and API changes

**New types:**

```go
type WorktreeGroup struct {
    ParentDir  string         // the container folder path
    ParentName string         // basename of container folder
    Worktrees  []WorktreeEntry
}

type WorktreeEntry struct {
    Path   string  // worktree root path
    Branch string  // branch name
    Name   string  // basename of worktree dir
}
```

**Changes to `CachedSession`:**

```go
type CachedSession struct {
    // ... existing fields unchanged ...

    WorktreeParent     string `json:"worktree_parent,omitempty"`      // parent folder path, if part of a worktree group
    WorktreeParentName string `json:"worktree_parent_name,omitempty"` // parent folder basename (for display)
}
```

**Changes to `SessionResponse`:**

```go
type SessionResponse struct {
    // ... existing fields unchanged ...

    WorktreeParent     string `json:"worktree_parent,omitempty"`      // parent folder path
    WorktreeParentName string `json:"worktree_parent_name,omitempty"` // parent folder basename (for display)
}
```

**Changes to `GET /api/sessions`:** Populate the new fields during response building. The frontend uses `worktree_parent_name` to decide whether to render nested.

**Changes to `GET /api/projects`:**

```go
type ProjectInfo struct {
    Path       string          `json:"path"`
    Name       string          `json:"name"`
    Branch     string          `json:"branch"`
    // New:
    IsWorktreeGroup bool            `json:"is_worktree_group,omitempty"`
    ParentPath      string          `json:"parent_path,omitempty"`       // set on worktree children
    ParentName      string          `json:"parent_name,omitempty"`       // set on worktree children
    Worktrees       []WorktreeEntry `json:"worktrees,omitempty"`         // set on discovered groups
}
```

### 4. Session Viewer Backend: New `POST /api/worktrees` endpoint

```
POST /api/worktrees
{
    "parent_path": "/Users/aj/code/myproject",
    "branch_name": "my-feature"
}
```

**Implementation:**

```go
func createWorktree(c echo.Context) error {
    // Parse body
    parentPath := body.ParentPath
    branchName := body.BranchName

    // Validate: parent must contain at least one discovered worktree
    // (Find any git worktree in a child dir to run git commands against)
    gitDir := findAnyGitChildDir(parentPath)
    if gitDir == "" {
        return error("no git worktree found in parent directory")
    }

    // Create worktree as sibling
    worktreePath := filepath.Join(parentPath, branchName)
    err := exec("git", "-C", gitDir, "worktree", "add", "-b", branchName, worktreePath)
    if err → return error

    // Create .clyde/sessions/ so the viewer discovers it
    os.MkdirAll(filepath.Join(worktreePath, ".clyde", "sessions"), 0755)

    // Trigger rescan
    go backgroundScan()

    return { "status": "ok", "path": worktreePath, "branch": branchName }
}
```

**Error cases:**
- Branch already exists → git error, return to frontend
- Directory already exists → git error, return to frontend
- No git worktree in parent → error
- Branch name validation (no spaces, etc.) → validate before calling git

### 5. Session Viewer Backend: Parent folder session discovery

The parent folder may have `.clyde/sessions/` too. During `backgroundScan()`, when we detect a worktree group, also check `<parentDir>/.clyde/sessions/` and include those sessions in the scan. These sessions get `Project = parentName` and `WorktreeParent = parentPath` (they are parent-level sessions).

The parent folder is added to the discovery set even if it wasn't originally found by `discoverProjectDirs()`.

### 6. Session Viewer Backend: worktree-aware project discovery enhancement

When a worktree group is detected (from any one member), add **all** worktree paths from `git worktree list` to the discovery set — even if they don't have `.clyde/` yet. This means discovering one worktree automatically discovers all siblings.

These "empty" worktrees (no `.clyde/sessions/`) appear in the sidebar as children with 0 sessions, ready for the user to "+" a session into.

### 7. Session Viewer Frontend: Two-level sidebar grouping

The current sidebar groups sessions flat by project name. The new sidebar needs a two-level hierarchy for worktree projects.

**New `groupedSessions` logic:**

```javascript
get groupedSessions() {
    const groups = {};
    const worktreeGroups = {};  // parentName → { ... }

    for (const s of this.visibleSessions) {
        if (s.worktree_parent_name) {
            // This session belongs to a worktree group
            if (!worktreeGroups[s.worktree_parent_name]) {
                worktreeGroups[s.worktree_parent_name] = {
                    parentName: s.worktree_parent_name,
                    parentPath: s.worktree_parent,
                    parentSessions: [],
                    children: {}
                };
            }
            const wg = worktreeGroups[s.worktree_parent_name];
            if (s.cwd === s.worktree_parent) {
                wg.parentSessions.push(s);
            } else {
                if (!wg.children[s.project]) {
                    wg.children[s.project] = { branch: s.branch, cwd: s.cwd, sessions: [] };
                }
                wg.children[s.project].sessions.push(s);
            }
        } else {
            // Normal project
            if (!groups[s.project]) groups[s.project] = { sessions: [] };
            groups[s.project].sessions.push(s);
        }
    }

    // Merge into final sorted list...
}
```

**Key UI rules:**
- Non-worktree projects render exactly as today (flat list of sessions under project header)
- Worktree-group projects get an outer header (the parent folder name) with inner sub-headers (each worktree, labeled by branch name), each collapsible
- The main worktree is shown among the children — it's just another worktree with its branch name, not special-cased
- Parent-level sessions (if any) appear directly under the group header, above the worktree children
- If a worktree group is detected, always show nested (even with only one worktree), so the user can create new worktrees from the parent "+" button

### 8. Session Viewer Frontend: "+" button behavior

| Context | Click "+" | Result |
|---------|-----------|--------|
| Normal project header | Direct click | Creates session (existing behavior) |
| Worktree group parent header | Shows dropdown | **"New Session"**: creates session in parent dir CWD |
|                               |                | **"New Worktree"**: opens worktree creation modal |
| Worktree child header | Direct click | Creates session in that worktree's CWD |

### 9. Session Viewer Frontend: Worktree creation modal

Minimal modal:
```
┌─────────────────────────────────┐
│  New Worktree                   │
│                                 │
│  Branch name: [____________]    │
│                                 │
│  Will create:                   │
│    Branch: my-feature           │
│    Path:   ~/code/myproject/    │
│            my-feature           │
│                                 │
│         [Cancel]  [Create]      │
└─────────────────────────────────┘
```

The path preview updates as the user types. The path is `<parent_dir>/<branch_name>`.

On success: rescan fires, new worktree appears in sidebar (with 0 sessions), user clicks "+" on it to create their first session.

---

## Edge Cases

| Case | Behavior |
|------|----------|
| Worktrees not in sibling layout (scattered paths) | No grouping — each treated as a separate project. Works fine, just no nesting. |
| Main worktree has no `.clyde/` | Still discovered via sibling detection. Appears in sidebar with 0 sessions. |
| Parent folder has no `.clyde/` | Still appears as group header (from worktree detection). "+" creates sessions there. |
| Worktree deleted after sessions created | Sessions persist on disk. Project appears in sidebar (still has `.clyde/`). Git operations may fail but sessions are readable. |
| Single worktree (main only, no linked) with worktree group detected | Renders nested so user can create new worktrees via parent "+". |
| User creates worktree from TUI (not SV) | SV discovers it on next scan (if it has `.clyde/`, or if a sibling is already known). |
| Name collision (parent dir and main worktree same basename) | Parent shows as group header name; main worktree shows as its branch name inside the group. No collision in the rendered UI. |

## Deferred: Discovery Scope

`discoverProjectDirs()` currently hardcodes `~/code` and `~/Downloads` as scan roots. This is machine-specific. A more generic solution (scanning `~/*` to a shallow depth, or using a config file for scan roots) is needed but is a **separate workstream** — it affects all projects, not just worktrees.
