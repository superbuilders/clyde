# Bug: Session Viewer doesn't detect live status after cross-user resume

**Filed:** 2026-07-17
**Severity:** Medium — live status indicator broken, but session itself works fine
**Component:** session-viewer

## Summary

When the Session Viewer launches clyde via tmux for a session that then gets renamed by cross-user resume (`CopyForResume`), the tmux session name no longer matches the new session directory name. The viewer can't find the tmux session and reports the session as offline even though clyde is actively running.

## Reproduction

1. User A (`test-user`) creates a session: `2026-07-17T14-05-04_test-user`
2. In the Session Viewer, User B (`thisistheaj`) clicks "Send" on that session
3. The viewer calls `startClyde()` which creates tmux session: `sv_2026-07-17T14-05-04_test-user`
4. Inside tmux, `clyde -r 2026-07-17T14-05-04_test-user` detects a cross-user resume and calls `CopyForResume`, creating a new directory: `2026-07-17T14-05-04_thisistheaj_from_2026-07-17T14-05-04_test-user`
5. The next background scan picks up the new directory as session ID `2026-07-17T14-05-04_thisistheaj_from_2026-07-17T14-05-04_test-user`
6. The viewer computes `tmuxName()` → `sv_2026-07-17T14-05-04_thisistheaj_from_2026-07-17T14-05-04_test-user`
7. **Mismatch:** the actual tmux session is still named `sv_2026-07-17T14-05-04_test-user`
8. `isTmuxRunning()` returns false → session shows as offline

## Root Cause

`tmuxName()` derives the tmux session name from the session directory name. But `CopyForResume` creates a *new* directory with a different name (`_from_` suffix) after the tmux session has already been created with the original name. The viewer has no mechanism to track this rename.

## Affected Scenarios

- **Regular projects:** User B resumes User A's session from the viewer → live indicator broken
- **Worktree projects:** Same scenario, but within a worktree group — the session appears under the correct worktree sub-header but shows as offline
- **Any cross-user resume initiated from the Session Viewer** (terminal `clyde -r` is unaffected since there's no tmux tracking)

## Proposed Fix

### Option A: Viewer renames tmux session after detecting a `_from_` directory (simplest)

In `backgroundScan()`, after discovering sessions, check if any newly-found `_from_` session has a tmux session running under the source session's name. If so, rename the tmux session:

```go
// In backgroundScan(), after session discovery loop:
for _, s := range cache.Sessions {
    if !strings.Contains(s.ID, "_from_") {
        continue
    }
    // Extract the source session ID from the _from_ suffix
    if idx := strings.Index(s.ID, "_from_"); idx >= 0 {
        sourceID := s.ID[idx+len("_from_"):]
        oldTmux := tmuxName(sourceID)
        newTmux := tmuxName(s.ID)
        if isTmuxRunning(oldTmux) && !isTmuxRunning(newTmux) {
            exec.Command("tmux", "rename-session", "-t", oldTmux, newTmux).Run()
        }
    }
}
```

### Option B: Viewer tracks tmux→session mapping independently

Maintain a map of `tmuxSessionName → sessionID` that gets updated when sessions are started, and check both the expected name and any legacy mappings during status checks.

### Option C: clyde itself renames the tmux session after CopyForResume

In `cli/cli.go`, after `CopyForResume` succeeds, check if we're running inside a viewer-managed tmux session and rename it:

```go
if sessionOwner != currentUser {
    sessionDir, err = session.CopyForResume(sessionDir, sessionsRoot, currentUser)
    // ... error handling ...

    // If running inside a viewer-managed tmux session, rename it to match
    if tmuxName := os.Getenv("TMUX"); tmuxName != "" {
        newName := "sv_" + filepath.Base(sessionDir)
        exec.Command("tmux", "rename-session", newName).Run()
    }
}
```

### Recommendation

**Option A** is the safest — it's contained in the session viewer, doesn't touch the TUI code path, and handles the rename retroactively during the next scan cycle (≤60s delay, or immediate on next poll). Option C is more precise but couples the TUI to the viewer's naming convention.

## Workaround

Manually rename the tmux session:
```bash
tmux rename-session -t sv_<old-session-id> sv_<new-session-id>
```

## Test Plan

1. **Unit test:** `tmuxName()` produces correct names for `_from_` session IDs
2. **E2E test (regular project):** Start a session as user A from the viewer, verify it shows live. Simulate cross-user resume (create `_from_` directory, rename tmux session via the fix). Verify live status persists.
3. **E2E test (worktree project):** Same as above but with the session inside a worktree. Verify the session still appears under the correct worktree group header AND shows live status.
4. **Regression test:** Normal same-user sessions continue to show live status correctly.
