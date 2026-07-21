Agent fixes:
- [x] Tool calls that flood the context (`ff7d47c`)
  - although we have automatic compaction over a certain threshold, when a tool call brings back a lot of data and it, by itself is larger than the context window (or even the remaining context window), the harness tries to send it in the subsequent LLM call and the loop crashes; instead, we should be able to detect this ahead of time and replace the tool result with an appropriate error message *before* sending, so that the receiving LLM understands what happened and can either adjust their query or figure out how to write the same result to file for later greping (in the case of bash commands the LLM may know how to do this), that way the harness never crashes for context window reasons
  - **impl:** `guardSingleToolResult()` in `agent/agent.go` runs per-result before the output callback fires. Estimates tokens at ~3.5 chars/token; replaces any result exceeding remaining context budget or >50% of total context window with a descriptive error message suggesting alternatives. Session files now store only the error (~567B instead of e.g. 1.67MB). 7 unit tests in `tests/oversized_tool_result_test.go`.

- [x] System prompt for head, tail, count etc (`86dd467`)
  - to mitigate the issue above, we have been including text in our prompts warning the llm about the failure mode and suggesting use of head/tail/count type tools, that should also be in the system prompt to avoid the failure case above even when it is recoverable
  - example: "{do thing}; be careful pulling too much data from tool calls because the session will crash if the context window is overloaded - use count, head, and tail as necessary"
  - **impl:** Added `AVOID OVERSIZED TOOL RESULTS` section to `agent/prompts/system.txt` with concrete guidance on using head/tail/grep, redirecting to file, and narrowing searches.

- [x] Remove progress.md from system prompt (`207de49`)
  - we should not specify specific project managment files by name otherwise the harness will start creating them, unprompted
  - **impl:** Removed entire `DOCUMENTATION & MEMORY` section from `agent/prompts/system.txt`.

- [x] .png files should get saved to gitignored folder by default (`b41ab88`)
  - clyde keeps taking screenshots with playwright and then commiting them to repositories that it works on - we need to prevent this either in code or in the system prompt
  - **impl:** `RedirectScreenshotPath()` in `agent/mcp/register.go` intercepts `browser_take_screenshot` calls and redirects relative filenames to `.clyde/screenshots/` (added to `.gitignore`). Absolute paths left untouched. Directory auto-created. 6 unit tests in `tests/screenshot_redirect_test.go`.

Session Viewer Fixes
- [x] Save entered text when changing tabs
  - localstorage should be used to preserve text state in the text entry area so tht it persists when it is navigated away from for any reason; it should be scoped to chat though
- [x] Image input
  - there should be an image input next to the text entry that lets users upload images, "uploaded" images should be placed in the root of the project in question and the text "include ./image-name.ext\n" should be prepended to the next prompt so that clyde knows to use the image tool to include the "uploaded" image
- [x] Hidden/collapse/expand toggle 
  - instead of visible/invisible, the buttons at the top right of the chat should toggle through Hidden/collapse/expand, where
    - hidden: everything except debug info *shows* as 'n tool calls' '1 thinking trace' etc
    - collapsed: same as now (n lines per n tool calls) but "collapse all"; user can still uncollapse individual lines
    - expand: same as now, but collapsed sections should get expanded when toggled back to this as well; user can still collapse individual lines
  - also, user and agent output should never be hidden, those buttons can be removed
- [x] improved visibility through colors:
  - the user input and the agent output should have 'You:\n' and 'Clyde:\n' prepended before they are drawn, 
  - also those labels should have colors so that when scanning large sections it is easy to find them
  - also, thinking traces and tool call/results, when expanded should have unique colors 
  - (see the clyde TUI for what colors to use)
- [x] live toggle
  - in the same bar as the sort dropdown and date range filter, there should be a blue toggle switch that, when on causes all projects in the sidebar to *only* show live chats; off should show everythign 
- [x] read state 
  - use logalstorage to track "read state" for each chat; when a chat goes from agent working to 'You: ' it should be marked as unread and a small red dot should appear in the top right corner of that chat in the side bar; when this chat is opened the dot should disappear
- [x] hamburger menu
  - in the sidebar with the chats, each chat should have a hamburger menu in the top right where chat options can go; to start, the menu should have these options:
    - mark un/read (toggles read state, uses appropriate verb depending on status)
    - kill (only for running sessions, stops them, kills tmux)
    - rename (presents a popup to rename the session)
- [ ] Copy message
  - there should be a hamburger menu on each message in the session; one of the options should be "copy message" copies the message content to clipboard (as plaintext/.md)
- [ ] delete message
  - there should be a hamburger menu on each message in the session; one of the options should be "delete message" which hard deletes from disk; this should only be available for stopped sessions
- [ ] Filter by all of: live, sh, unread
  - instead of just a checkbox for 'live' there should be a dropdown (similar to what we have for projects), where i should be able to toggle a filter for 'live' (blue dot), 'sh' (green dot), or unread; non selected should show all, one selected=>just tht one, and multiple selected => the union of selected statuses (NOT intersection)
- [ ] Mark all as ‘read’ button
  - i should be able to 'mark all as read' from a hambuger/dropdown menu at any context level (worktree, project, or globally)
- [ ] Truly hidden diagnostics
  - when diagnostic-level messages are hidden they should show up nowhere, they should not even have '3 diagnostics as text' AND - nor should they interrupt chains of other message types (e.g. if a thread has '2 tool calls \n 3 diagnostics \n 3 tool calls' in part of its history, then that *whole* section should just show as '5 tool calls' when tools and diagnostics are hidden and '2 tool calls \n <the actual diagnostic logs> \n 3 tool calls', when tools are hidden and diagnostics unhidden)
- [ ] Ability to Delete/archive worktrees
  - in the hamburger menu for worktrees i should be able to delete them, if i do the .clyde/sessions from that worktree should be moved to the ./clyde/session under main/master (or whatever the primary worktree is)
- [ ] Unchecking projects does not work if they are worktree based projects
  - projects with worktrees should respect the project level toggle for visibilty (we don't need a worktree level toggle though)
- [ ] “Open in terminal” button => direct to tmux
  - in the hamburger menu for sessions (both in the sidebar AND in the top right corner of the detail view) there should be an option for 'open in terminal'; if there is a running tmux session we should open a new mac terminal and attach to that session; if there is none, we should create one first 
- [ ] Multiline text + many chars bug (Reproduce?)

- Tui fixes
- [ ] Back cursor movement with multiple lines
    - when we use multiple lines in the text entry and then go back so far that we go to a previous line, the redraw function appears to "delete" 1 to many lines - that is to say, the last line of output *above* the text entry prompt gets deleted upon every subsequent backcurosr (e.g. type 3 lines, move to the beginning of the 3rd line => works great, press back from the beginning of the 3rd line => moves to end of line 2 and deletes the last output line, press back 5 more times => now we are on the len-5 char of line 2 and the last 6 output lines have been deleted from the terminal)
- [ ] Redraw chat on -r
    - when we resume a chat with -r the entire previous message history shgould be output to the terminal with verbosity level respected
- [ ] Redraw chat on verbosity change
    - just like when we resume a chat, we should be able to use a command to change verbosity in the TUI; it should be imlemented so that "/verbosity <level>" has the same effect as: killing the chat, then calling 'clyde -r' with the new verbosity level passed in (*including* that the entire chat gets reemitted in the new verbosity)

