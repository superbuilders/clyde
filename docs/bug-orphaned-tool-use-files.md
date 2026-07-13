# BUG: Orphaned tool-use session files — missing tool-result for `include_file` and interrupted tools

**Date discovered:** 2026-07-02  
**Severity:** Medium (causes potential API errors on session resume)  
**Affected component:** `agent/agent.go` (output callback skip), `agent/session/resume.go` (reconstruction)

---

## Summary

Certain tool invocations write a `*_tool-use.md` session file but never write a corresponding `*_tool-result.md` file. This leaves orphaned tool-use records that can break session reconstruction and cause Claude API errors on `--resume`.

Two distinct root causes produce orphans:

### 1. `include_file` (image) — structural bug 🐛

**Root cause:** In `agent/agent.go`, the output callback (which writes `*_tool-result.md`) is intentionally skipped for image results:

```go
// agent.go ~line 579
if resultContent != "" && !strings.HasPrefix(resultContent, "Image loaded") {
    if a.outputCallback != nil {
        a.outputCallback(resultContent, toolBlock.ID)
    }
}
```

When `include_file` loads an image, `resultContent` is set to `"Image loaded successfully (image/png, 42.5 KB)"`, which matches the `HasPrefix` guard. The output callback is never called, so **no `*_tool-result.md` file is ever written**.

This means **every** `include_file` call for images produces an orphaned tool-use file. It's 100% reproducible and systematic.

**Impact on resume:** During `ReconstructHistory`, the orphaned tool-use file adds a `tool_use` content block to the assistant message, but no corresponding `tool_result` is ever added to the user message. The Claude API requires every `tool_use` to have a matching `tool_result`. This can cause:
- API validation errors (`400 Bad Request`) on resumed sessions
- Malformed conversation history with unmatched tool_use/tool_result pairs

### 2. Session interruption — expected but unhandled 🔧

When the user kills a session (Ctrl-C, terminal close, crash) while a tool is executing, the tool-use file has already been written (it's written at dispatch time via `WithToolUseCallback`), but the result never comes back. This is inherent to async tool execution and expected behavior, but the orphaned files should be cleaned up.

**Observed in the wild:** In session `2026-07-01T14-49-58_thisistheaj`, 4 `run_bash` calls were orphaned — all were AWS/env-related commands where the session was interrupted mid-execution.

---

## Evidence

Session `2026-07-01T14-49-58_thisistheaj`: **39 tool-use files, 34 tool-result files → 5 orphans**

| File | Tool | Root Cause |
|------|------|------------|
| `2026-07-01T14-50-21.862_tool-use.md` | `include_file` (cale-2.png) | Output callback skipped for images |
| `2026-07-01T14-51-02.045_tool-use.md` | `run_bash` (grep .env) | Session interrupted |
| `2026-07-01T15-01-55.616_tool-use.md` | `run_bash` (grep .env) | Session interrupted |
| `2026-07-01T15-02-01.183_tool-use.md` | `run_bash` (grep .env) | Session interrupted |
| `2026-07-01T15-02-52.724_tool-use.md` | `run_bash` (aws login &) | Session interrupted (last file in session) |

---

## Suggested Fixes

### Fix 1: Write tool-result for `include_file` images

In `agent/agent.go`, ensure the output callback fires for image results. The callback should receive the human-readable confirmation string (not the base64 data):

```go
// After setting resultContent = "Image loaded successfully..."
// DON'T skip the output callback — it's needed for session persistence
if a.outputCallback != nil {
    a.outputCallback(resultContent, toolBlock.ID)
}
```

The display-layer can still suppress the visual output if desired, but the session file must be written.

### Fix 2: Prune orphans on session resume

In `ReconstructHistory` (or as a pre-pass in `Open`), detect tool-use files with no matching tool-result and either:
- **Option A:** Skip them during reconstruction (drop the orphaned `tool_use` block)
- **Option B:** Synthesize a placeholder `tool_result` (e.g., `"[Tool result unavailable — session was interrupted]"`)

Option B is more robust because it maintains the tool_use/tool_result pairing the API expects.

### Fix 3: Cleanup script (stopgap)

A utility script `scripts/clyde-prune-orphans.sh` has been added to manually clean orphaned tool-use files from sessions. Usage:

```bash
./scripts/clyde-prune-orphans.sh <session-name> --dry-run   # preview
./scripts/clyde-prune-orphans.sh <session-name>              # delete orphans
```

The script matches tool-use and tool-result files by their `[toolu_*]` IDs and deletes any tool-use files that have no corresponding result.

---

## Reproduction

```bash
# Start a session and ask clyde to include an image
clyde "look at screenshot.png"

# Check the session — the include_file tool-use will have no matching result
ls .clyde/sessions/<latest-session>/*tool-use* | wc -l
ls .clyde/sessions/<latest-session>/*tool-result* | wc -l
# tool-use count will be > tool-result count

# Or use the cleanup script to find them
./scripts/clyde-prune-orphans.sh <session-name> --dry-run
```
