# CMP Manual Testing Playbook

Manual verification of CMP-1/CMP-2/CMP-3 features via the Clyde TUI.

> **Prerequisites**: A working `~/.clyde/config` with a valid `TS_AGENT_API_KEY`.
> All tests use a temporary config override to set aggressive compaction thresholds.

---

## Setup: Temporary Config for Early Compaction

The default compaction threshold is 184,000 tokens (200k context − 16k reserve), which requires a very long conversation to trigger. For manual testing, create a temporary config that triggers compaction after just 1-2 exchanges.

```bash
# Backup your real config
cp ~/.clyde/config ~/.clyde/config.bak

# Create test config with aggressive thresholds
cat > ~/.clyde/config << 'EOF'
TS_AGENT_API_KEY=<your-real-key>

# CMP testing: trigger compaction after ~1-2 exchanges
# contextWindowSize is hardcoded at 200000, so:
#   threshold = 200000 - 195000 = 5000 tokens
#   system prompt + tools ≈ 4000-5000 tokens
#   first exchange pushes past 5000 → compaction triggers on turn 2
RESERVE_TOKENS=195000

# CMP-3 testing: lower threshold so normal tool results trigger summarization
TOOL_RESULT_THRESHOLD=500
EOF
```

After testing, restore:
```bash
cp ~/.clyde/config.bak ~/.clyde/config
```

---

## Test 1: CMP-1 — Automatic Compaction Trigger

**Goal**: Verify compaction fires automatically when the context threshold is exceeded.

### Steps

1. **Start a REPL session**:
   ```bash
   ./clyde
   ```

2. **Send a first message** (creates the conversation):
   ```
   You: What is 2+2?
   ```
   - ✅ Claude responds normally (no compaction yet — this is the first API call)
   - ✅ The prompt line shows a context % (should be low, ~2-3%)

3. **Send a second message** (triggers compaction):
   ```
   You: Now what is 10+10?
   ```
   - ✅ **Compaction marker appears**: `🗜️ Compacting conversation history...`
   - ✅ **Phase progress lines appear** (5 lines, one per phase):
     ```
     🗜️ Compaction phase 1/5: extracting goals & constraints...
     🗜️ Compaction phase 2/5: capturing decisions...
     🗜️ Compaction phase 3/5: analyzing file & git state...
     🗜️ Compaction phase 4/5: synthesizing tool outputs...
     🗜️ Compaction phase 5/5: drafting handoff document...
     ```
   - ✅ Claude eventually responds to the question (conversation continues seamlessly)
   - ✅ The context % on the prompt drops significantly after compaction

4. **Verify the session persisted compaction files**:
   ```
   You: exit
   ```
   Note the session path printed (e.g., `Session saved: .clyde/sessions/2026-...`).
   ```bash
   ls .clyde/sessions/<your-session>/*_compaction.md
   ls .clyde/sessions/<your-session>/*_system.md
   cat .clyde/sessions/<your-session>/*_compaction.md
   cat .clyde/sessions/<your-session>/*_system.md
   ```
   - ✅ A `*_compaction.md` file exists containing `🗜️`
   - ✅ A `*_system.md` file exists containing `**System:**` and the handoff document

### Expected Behavior
- Compaction fires silently between turns — the user sees progress lines but no interruption
- The conversation continues normally after compaction
- The first user message is preserved in the handoff (check `*_system.md`)

---

## Test 2: CMP-2 — Multi-Phase Handoff Quality

**Goal**: Verify the 5-phase workflow produces a structured, high-quality handoff document with git state.

### Steps

1. **Start a REPL with debug logging**:
   ```bash
   ./clyde --debug
   ```

2. **Have a multi-topic conversation** (at least 2 turns before compaction triggers):
   ```
   You: I want to build a CLI tool in Go that converts CSV files to JSON.
         It should support filtering by column and sorting.
   ```
   Wait for response.
   ```
   You: Add cobra for CLI flags and implement the filtering feature.
   ```
   - ✅ Compaction triggers (due to aggressive threshold)
   - ✅ At `--debug` level, you see **intermediate phase outputs** (truncated to 500 chars each):
     ```
     🗜️ Phase 1 output:
     [goals and constraints extracted...]

     🗜️ Phase 2 output:
     [decisions captured...]

     🗜️ Phase 3 output:
     [file state with git info...]

     🗜️ Phase 4 output:
     [tool result synthesis...]

     🗜️ Phase 5 output (final handoff):
     [structured handoff document...]
     ```

3. **Examine the handoff document** (from the session file):
   ```
   You: exit
   ```
   ```bash
   cat .clyde/sessions/<your-session>/*_system.md
   ```
   - ✅ Contains `## Goal` section
   - ✅ Contains `## Constraints` section
   - ✅ Contains `## Progress` section
   - ✅ Contains `## Key Decisions` section
   - ✅ Contains `## Current State` section **with git branch and commit SHA**
   - ✅ Contains `## Next Steps` section
   - ✅ Contains `## Critical Context` section
   - ✅ If you have uncommitted changes: contains `⚠️ Uncommitted changes detected`

### Expected Behavior
- The handoff reads like a developer status update, not a lossy summary
- Git state (branch, SHA, commit message) is referenced in Current State
- No raw diffs in the document — just commit references

---

## Test 3: CMP-2 — Git State in Non-Repo

**Goal**: Verify graceful behavior when not inside a git repository.

### Steps

1. **Navigate to a non-git temp directory**:
   ```bash
   cd /tmp
   ./path/to/clyde
   ```

2. **Trigger compaction** (same as Test 1 — send 2 messages):
   ```
   You: Hello, tell me about Go programming.
   ```
   ```
   You: What are goroutines?
   ```
   - ✅ Compaction triggers and completes without errors
   - ✅ No crash or error about missing git
   - ✅ The handoff document notes "not a git repo" (or simply omits git state)

---

## Test 4: CMP-3 — Intelligent Tool-Result Summarization

**Goal**: Verify that large tool outputs are summarized intelligently (not hard-truncated) during compaction.

### Setup
Ensure `TOOL_RESULT_THRESHOLD=500` in `~/.clyde/config` (from the setup above).

### Steps

1. **Start a REPL with debug logging**:
   ```bash
   ./clyde --debug
   ```

2. **Generate a large tool output**:
   ```
   You: Search for all function definitions in Go files in this project
   ```
   - This triggers `grep` which will likely return >500 chars of output
   - Claude responds with analysis

3. **Send another message to trigger compaction**:
   ```
   You: Now summarize what you found.
   ```
   - ✅ Compaction triggers
   - ✅ At debug level, you see: `🗜️ Summarized tool result: NNNN chars → MMM chars`
   - ✅ The summarization diagnostic shows the original and summarized sizes
   - ✅ If the LLM summarization fails for any reason, you see:
     `🗜️ Tool result summarization failed, falling back to truncation: ...`

4. **Examine the handoff**:
   ```
   You: exit
   ```
   ```bash
   cat .clyde/sessions/<your-session>/*_system.md
   ```
   - ✅ The handoff mentions the grep results intelligently (not just first 500 chars)
   - ✅ Key function names and file paths from the grep output are preserved
   - ✅ If summarization ran, there should be no `... (truncated)` markers in the handoff for that output

---

## Test 5: CMP-1 — First User Message Preservation

**Goal**: Verify the "sacred" first message survives compaction verbatim.

### Steps

1. **Start a REPL**:
   ```bash
   ./clyde
   ```

2. **Send a distinctive first message**:
   ```
   You: MISSION CRITICAL: Build a REST API in Go with exactly 3 endpoints:
        GET /health, POST /data, DELETE /data/:id.
        Must use chi router, not gin. Must have 95% test coverage.
   ```

3. **Continue conversation to trigger compaction**:
   ```
   You: Start with the health endpoint.
   ```

4. **After compaction completes, ask Claude what the original mission was**:
   ```
   You: What was my original request? Quote it exactly.
   ```
   - ✅ Claude should be able to quote the original mission verbatim
   - ✅ It should include "MISSION CRITICAL", "chi router", "95% test coverage"

5. **Also verify in session files**:
   ```
   You: exit
   ```
   ```bash
   # The first user.md should be the original mission
   cat .clyde/sessions/<your-session>/*_user.md | head -10
   
   # The system.md should NOT contain the original mission
   # (it's preserved separately, not summarized into the handoff)
   ```

---

## Test 6: Session Resume After Compaction

**Goal**: Verify that `--resume` correctly loads from the post-compaction `*_system.md`.

### Steps

1. **Complete Test 1 or Test 2** (a session with compaction).

2. **Resume the session**:
   ```bash
   ./clyde --resume
   ```
   - ✅ Shows: `Resuming session: <session-id> (N messages loaded)`
   - ✅ The loaded message count should reflect the *compacted* history (small), not the full pre-compaction count

3. **Ask about previous context**:
   ```
   You: What have we been working on?
   ```
   - ✅ Claude has context from the compaction summary
   - ✅ Conversation continues coherently

4. **Exit and check the session directory**:
   ```
   You: exit
   ```
   ```bash
   ls .clyde/sessions/<your-session>/
   ```
   - ✅ New message files were appended after the compaction files
   - ✅ The `*_system.md` file is still present (not overwritten)

---

## Test 7: CMP-2 — `COMPACT_INCLUDE_RECENT_CONTEXT` Flag

**Goal**: Verify the flag controls whether recent messages feed into compaction phases.

### Steps

1. **Add the flag to config**:
   ```bash
   echo "COMPACT_INCLUDE_RECENT_CONTEXT=false" >> ~/.clyde/config
   ```

2. **Run a session that triggers compaction** (same as Test 1).

3. **Observe behavior**:
   - ✅ Compaction still works (doesn't crash or error)
   - ✅ The handoff document may be slightly less contextual (no bridging of recent messages)
   - ✅ This is a "max token savings" mode — fewer tokens spent on compaction

4. **Clean up**:
   ```bash
   # Remove the line or restore backup
   cp ~/.clyde/config.bak ~/.clyde/config
   ```

---

## Test 8: Compaction Visibility at Different Log Levels

**Goal**: Verify compaction output respects log level gating.

| Level | Expected |
|-------|----------|
| `--silent` | Nothing visible — compaction happens silently |
| `-q` / `--quiet` | `🗜️` marker and phase progress lines visible |
| *(default/normal)* | Same as quiet (compaction lines are progress-level) |
| `-v` / `--verbose` | Same as normal + cache diagnostics |
| `--debug` | All of the above + intermediate phase outputs + summarization stats |

### Steps

1. **Test at quiet level**:
   ```bash
   ./clyde -q
   ```
   Send 2 messages, verify `🗜️` lines appear.

2. **Test at debug level**:
   ```bash
   ./clyde --debug
   ```
   Send 2 messages, verify intermediate phase outputs appear (Phase 1-5 debug blocks).

3. **Test at silent level**:
   ```bash
   ./clyde --silent
   ```
   Send 2 messages, verify NO compaction output appears (but conversation still works).

---

## Teardown

```bash
# Restore original config
cp ~/.clyde/config.bak ~/.clyde/config

# Clean up test sessions (optional)
ls -la .clyde/sessions/
# rm -rf .clyde/sessions/<test-session-dirs>
```

---

## Quick Smoke Test (5 minutes)

If you only have time for one test, do this:

```bash
# Aggressive compaction config
cat > /tmp/clyde-test-config << 'EOF'
TS_AGENT_API_KEY=<your-key>
RESERVE_TOKENS=195000
TOOL_RESULT_THRESHOLD=500
EOF
cp ~/.clyde/config ~/.clyde/config.bak
cp /tmp/clyde-test-config ~/.clyde/config

# Run with debug logging
./clyde --debug

# Type these two messages:
#   1) "List all Go files in this project"  (triggers a tool + gets a large result)
#   2) "What patterns do you see?"          (triggers compaction)
#
# Verify:
#   ✅ 🗜️ Compacting conversation history... appears
#   ✅ 5 phase progress lines appear
#   ✅ Phase debug output visible (goals, decisions, files, tools, handoff)
#   ✅ Tool result summarization diagnostic visible
#   ✅ Claude responds coherently after compaction
#   ✅ exit → Session saved message → *_compaction.md and *_system.md exist

# Restore
cp ~/.clyde/config.bak ~/.clyde/config
```
